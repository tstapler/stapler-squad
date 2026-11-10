"use strict";
exports.id = 3589;
exports.ids = [3589];
exports.modules = {

/***/ 36694:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._1yfvjih0 {\n  background: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz38);\n  padding: var(--_19ez2rz31) var(--_19ez2rz31);\n}\n._1yfvjih1 {\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3h);\n  color: var(--_19ez2rz0);\n  margin-bottom: var(--_19ez2rz30);\n}\n._1yfvjih2 {\n  width: 100%;\n  height: 220px;\n}\n._1yfvjih3 {\n  display: flex;\n  align-items: center;\n  justify-content: center;\n  height: 220px;\n  color: var(--_19ez2rz2);\n  font-size: var(--_19ez2rz3b);\n}\n._1yfvjih4 {\n  display: flex;\n  flex-wrap: wrap;\n  gap: var(--_19ez2rz2y) var(--_19ez2rz30);\n  margin-top: var(--_19ez2rz2z);\n}\n._1yfvjih5 {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2y);\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz1);\n}\n._1yfvjih6 {\n  width: 8px;\n  height: 8px;\n  border-radius: var(--_19ez2rz39);\n  flex-shrink: 0;\n}\n._1yfvjih7 {\n  color: var(--_19ez2rzz);\n  font-style: italic;\n}\n._1yfvjih8 {\n  color: var(--_19ez2rz2);\n  font-style: italic;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 41258:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  yP: () => (/* binding */ Bar),
  LP: () => (/* binding */ computeBarRectangles)
});

// UNUSED EXPORTS: defaultBarProps

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/clsx@2.1.1/node_modules/clsx/dist/clsx.mjs
var clsx = __webpack_require__(37595);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/container/Layer.js
var Layer = __webpack_require__(44084);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/component/Cell.js
var Cell = __webpack_require__(68213);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/component/LabelList.js
var LabelList = __webpack_require__(65379);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/DataUtils.js
var DataUtils = __webpack_require__(36223);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/ReactUtils.js
var ReactUtils = __webpack_require__(75976);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/ChartUtils.js + 6 modules
var ChartUtils = __webpack_require__(26533);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/types.js
var types = __webpack_require__(4155);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/tiny-invariant@1.3.3/node_modules/tiny-invariant/dist/esm/tiny-invariant.js
var isProduction = "production" === 'production';
var prefix = 'Invariant failed';
function invariant(condition, message) {
    if (condition) {
        return;
    }
    if (isProduction) {
        throw new Error(prefix);
    }
    var provided = typeof message === 'function' ? message() : message;
    var value = provided ? "".concat(prefix, ": ").concat(provided) : prefix;
    throw new Error(value);
}



// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/ActiveShapeUtils.js + 17 modules
var ActiveShapeUtils = __webpack_require__(5800);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/BarUtils.js
function _extends() {
    return _extends = Object.assign ? Object.assign.bind() : function(n) {
        for(var e = 1; e < arguments.length; e++){
            var t = arguments[e];
            for(var r in t)({}).hasOwnProperty.call(t, r) && (n[r] = t[r]);
        }
        return n;
    }, _extends.apply(null, arguments);
}




function BarRectangle(props) {
    return /*#__PURE__*/ react.createElement(ActiveShapeUtils/* Shape */.y, _extends({
        shapeType: "rectangle",
        activeClassName: "recharts-active-bar",
        inActiveClassName: "recharts-inactive-bar"
    }, props));
}
/**
 * Safely gets minPointSize from the minPointSize prop if it is a function
 * @param minPointSize minPointSize as passed to the Bar component
 * @param defaultValue default minPointSize
 * @returns minPointSize
 */ var minPointSizeCallback = function minPointSizeCallback(minPointSize) {
    var defaultValue = arguments.length > 1 && arguments[1] !== undefined ? arguments[1] : 0;
    return (value, index)=>{
        if ((0,DataUtils/* isNumber */.Et)(minPointSize)) return minPointSize;
        var isValueNumberOrNil = (0,DataUtils/* isNumber */.Et)(value) || (0,DataUtils/* isNullish */.uy)(value);
        if (isValueNumberOrNil) {
            return minPointSize(value, index);
        }
        !isValueNumberOrNil ?  true ? invariant(false, "minPointSize callback function received a value with type of ".concat(typeof value, ". Currently only numbers or null/undefined are supported.")) : 0 : void 0;
        return defaultValue;
    };
};

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/hooks.js
var hooks = __webpack_require__(53151);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/tooltipSlice.js
var tooltipSlice = __webpack_require__(984);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/context/tooltipContext.js


/**
 * Some graphical items choose to provide more information to the tooltip
 * and some do not.
 */ var useMouseEnterItemDispatch = (onMouseEnterFromProps, dataKey, graphicalItemId)=>{
    var dispatch = (0,hooks/* useAppDispatch */.j)();
    return (data, index)=>(event)=>{
            onMouseEnterFromProps === null || onMouseEnterFromProps === void 0 || onMouseEnterFromProps(data, index, event);
            dispatch((0,tooltipSlice/* setActiveMouseOverItemIndex */.RD)({
                activeIndex: String(index),
                activeDataKey: dataKey,
                activeCoordinate: data.tooltipPosition,
                activeGraphicalItemId: graphicalItemId
            }));
        };
};
var useMouseLeaveItemDispatch = (onMouseLeaveFromProps)=>{
    var dispatch = (0,hooks/* useAppDispatch */.j)();
    return (data, index)=>(event)=>{
            onMouseLeaveFromProps === null || onMouseLeaveFromProps === void 0 || onMouseLeaveFromProps(data, index, event);
            dispatch((0,tooltipSlice/* mouseLeaveItem */.oP)());
        };
};
var useMouseClickItemDispatch = (onMouseClickFromProps, dataKey, graphicalItemId)=>{
    var dispatch = (0,hooks/* useAppDispatch */.j)();
    return (data, index)=>(event)=>{
            onMouseClickFromProps === null || onMouseClickFromProps === void 0 || onMouseClickFromProps(data, index, event);
            dispatch((0,tooltipSlice/* setActiveClickItemIndex */.ML)({
                activeIndex: String(index),
                activeDataKey: dataKey,
                activeCoordinate: data.tooltipPosition,
                activeGraphicalItemId: graphicalItemId
            }));
        };
};

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/SetTooltipEntrySettings.js
var SetTooltipEntrySettings = __webpack_require__(25019);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/context/ErrorBarContext.js
var ErrorBarContext = __webpack_require__(23991);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/cartesian/GraphicalItemClipPath.js
var GraphicalItemClipPath = __webpack_require__(71499);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/context/chartLayoutContext.js
var chartLayoutContext = __webpack_require__(87644);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/reselect@5.1.1/node_modules/reselect/dist/reselect.mjs
var reselect = __webpack_require__(48559);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/axisSelectors.js + 8 modules
var axisSelectors = __webpack_require__(66882);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/dataSelectors.js
var dataSelectors = __webpack_require__(92396);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/selectChartOffsetInternal.js + 1 modules
var selectChartOffsetInternal = __webpack_require__(19239);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/rootPropsSelectors.js
var rootPropsSelectors = __webpack_require__(82676);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/types/StackedGraphicalItem.js
var StackedGraphicalItem = __webpack_require__(32936);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/combiners/combineBarSizeList.js


var getBarSize = (globalSize, totalSize, selfSize)=>{
    var barSize = selfSize !== null && selfSize !== void 0 ? selfSize : globalSize;
    if ((0,DataUtils/* isNullish */.uy)(barSize)) {
        return undefined;
    }
    return (0,DataUtils/* getPercentValue */.F4)(barSize, totalSize, 0);
};
var combineBarSizeList = (allBars, globalSize, totalSize)=>{
    var initialValue = {};
    var stackedBars = allBars.filter(StackedGraphicalItem/* isStacked */.g);
    var unstackedBars = allBars.filter((b)=>b.stackId == null);
    var groupByStack = stackedBars.reduce((acc, bar)=>{
        var s = acc[bar.stackId];
        if (s == null) {
            s = [];
        }
        s.push(bar);
        acc[bar.stackId] = s;
        return acc;
    }, initialValue);
    var stackedSizeList = Object.entries(groupByStack).map((_ref)=>{
        var _bars$;
        var [stackId, bars] = _ref;
        var dataKeys = bars.map((b)=>b.dataKey);
        var barSize = getBarSize(globalSize, totalSize, (_bars$ = bars[0]) === null || _bars$ === void 0 ? void 0 : _bars$.barSize);
        return {
            stackId,
            dataKeys,
            barSize
        };
    });
    var unstackedSizeList = unstackedBars.map((b)=>{
        var dataKeys = [
            b.dataKey
        ].filter((dk)=>dk != null);
        var barSize = getBarSize(globalSize, totalSize, b.barSize);
        return {
            stackId: undefined,
            dataKeys,
            barSize
        };
    });
    return [
        ...stackedSizeList,
        ...unstackedSizeList
    ];
};

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/isWellBehavedNumber.js
var isWellBehavedNumber = __webpack_require__(38490);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/combiners/combineAllBarPositions.js
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


function getBarPositions(barGap, barCategoryGap, bandSize, sizeList, maxBarSize) {
    var _sizeList$;
    var len = sizeList.length;
    if (len < 1) {
        return undefined;
    }
    var realBarGap = (0,DataUtils/* getPercentValue */.F4)(barGap, bandSize, 0, true);
    var result;
    var initialValue = [];
    // whether is barSize set by user
    // Okay but why does it check only for the first element? What if the first element is set but others are not?
    if ((0,isWellBehavedNumber/* isWellBehavedNumber */.H)((_sizeList$ = sizeList[0]) === null || _sizeList$ === void 0 ? void 0 : _sizeList$.barSize)) {
        var useFull = false;
        var fullBarSize = bandSize / len;
        var sum = sizeList.reduce((res, entry)=>res + (entry.barSize || 0), 0);
        sum += (len - 1) * realBarGap;
        if (sum >= bandSize) {
            sum -= (len - 1) * realBarGap;
            realBarGap = 0;
        }
        if (sum >= bandSize && fullBarSize > 0) {
            useFull = true;
            fullBarSize *= 0.9;
            sum = len * fullBarSize;
        }
        var offset = (bandSize - sum) / 2 >> 0;
        var prev = {
            offset: offset - realBarGap,
            size: 0
        };
        result = sizeList.reduce((res, entry)=>{
            var _entry$barSize;
            var newPosition = {
                stackId: entry.stackId,
                dataKeys: entry.dataKeys,
                position: {
                    offset: prev.offset + prev.size + realBarGap,
                    size: useFull ? fullBarSize : (_entry$barSize = entry.barSize) !== null && _entry$barSize !== void 0 ? _entry$barSize : 0
                }
            };
            var newRes = [
                ...res,
                newPosition
            ];
            prev = newPosition.position;
            return newRes;
        }, initialValue);
    } else {
        var _offset = (0,DataUtils/* getPercentValue */.F4)(barCategoryGap, bandSize, 0, true);
        if (bandSize - 2 * _offset - (len - 1) * realBarGap <= 0) {
            realBarGap = 0;
        }
        var originalSize = (bandSize - 2 * _offset - (len - 1) * realBarGap) / len;
        if (originalSize > 1) {
            originalSize >>= 0;
        }
        var size = (0,isWellBehavedNumber/* isWellBehavedNumber */.H)(maxBarSize) ? Math.min(originalSize, maxBarSize) : originalSize;
        result = sizeList.reduce((res, entry, i)=>[
                ...res,
                {
                    stackId: entry.stackId,
                    dataKeys: entry.dataKeys,
                    position: {
                        offset: _offset + (originalSize + realBarGap) * i + (originalSize - size) / 2,
                        size
                    }
                }
            ], initialValue);
    }
    return result;
}
var combineAllBarPositions = (sizeList, globalMaxBarSize, barGap, barCategoryGap, barBandSize, bandSize, childMaxBarSize)=>{
    var maxBarSize = (0,DataUtils/* isNullish */.uy)(childMaxBarSize) ? globalMaxBarSize : childMaxBarSize;
    var allBarPositions = getBarPositions(barGap, barCategoryGap, barBandSize !== bandSize ? barBandSize : bandSize, sizeList, maxBarSize);
    if (barBandSize !== bandSize && allBarPositions != null) {
        allBarPositions = allBarPositions.map((pos)=>_objectSpread(_objectSpread({}, pos), {}, {
                position: _objectSpread(_objectSpread({}, pos.position), {}, {
                    offset: pos.position.offset - barBandSize / 2
                })
            }));
    }
    return allBarPositions;
};

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/stacks/getStackSeriesIdentifier.js
var getStackSeriesIdentifier = __webpack_require__(13662);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/combiners/combineStackedData.js

var combineStackedData = (stackGroups, barSettings)=>{
    var stackSeriesIdentifier = (0,getStackSeriesIdentifier/* getStackSeriesIdentifier */.x)(barSettings);
    if (!stackGroups || stackSeriesIdentifier == null || barSettings == null) {
        return undefined;
    }
    var { stackId } = barSettings;
    if (stackId == null) {
        return undefined;
    }
    var stackGroup = stackGroups[stackId];
    if (!stackGroup) {
        return undefined;
    }
    var { stackedData } = stackGroup;
    if (!stackedData) {
        return undefined;
    }
    return stackedData.find((sd)=>sd.key === stackSeriesIdentifier);
};

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/graphicalItemSelectors.js
var graphicalItemSelectors = __webpack_require__(52236);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/combiners/combineBarPosition.js
var combineBarPosition = (allBarPositions, barSettings)=>{
    if (allBarPositions == null || barSettings == null) {
        return undefined;
    }
    var position = allBarPositions.find((p)=>p.stackId === barSettings.stackId && barSettings.dataKey != null && p.dataKeys.includes(barSettings.dataKey));
    if (position == null) {
        return undefined;
    }
    return position.position;
};

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/barSelectors.js














var pickIsPanorama = (_state, _id, isPanorama)=>isPanorama;
var pickBarId = (_state, id)=>id;
var selectSynchronisedBarSettings = (0,reselect/* createSelector */.Mz)([
    axisSelectors/* selectUnfilteredCartesianItems */.ld,
    pickBarId
], (graphicalItems, id)=>graphicalItems.filter((item)=>item.type === 'bar').find((item)=>item.id === id));
var selectMaxBarSize = (0,reselect/* createSelector */.Mz)([
    selectSynchronisedBarSettings
], (barSettings)=>barSettings === null || barSettings === void 0 ? void 0 : barSettings.maxBarSize);
var pickCells = (_state, _id, _isPanorama, cells)=>cells;
var selectAllVisibleBars = (0,reselect/* createSelector */.Mz)([
    chartLayoutContext/* selectChartLayout */.fz,
    axisSelectors/* selectUnfilteredCartesianItems */.ld,
    graphicalItemSelectors/* selectXAxisIdFromGraphicalItemId */.W,
    graphicalItemSelectors/* selectYAxisIdFromGraphicalItemId */.B,
    pickIsPanorama
], (layout, allItems, xAxisId, yAxisId, isPanorama)=>allItems.filter((i)=>{
        if (layout === 'horizontal') {
            return i.xAxisId === xAxisId;
        }
        return i.yAxisId === yAxisId;
    }).filter((i)=>i.isPanorama === isPanorama).filter((i)=>i.hide === false).filter((i)=>i.type === 'bar'));
var selectBarStackGroups = (state, id, isPanorama)=>{
    var layout = (0,chartLayoutContext/* selectChartLayout */.fz)(state);
    var xAxisId = (0,graphicalItemSelectors/* selectXAxisIdFromGraphicalItemId */.W)(state, id);
    var yAxisId = (0,graphicalItemSelectors/* selectYAxisIdFromGraphicalItemId */.B)(state, id);
    if (xAxisId == null || yAxisId == null) {
        return undefined;
    }
    if (layout === 'horizontal') {
        return (0,axisSelectors/* selectStackGroups */.TC)(state, 'yAxis', yAxisId, isPanorama);
    }
    return (0,axisSelectors/* selectStackGroups */.TC)(state, 'xAxis', xAxisId, isPanorama);
};
var selectBarCartesianAxisSize = (state, id)=>{
    var layout = (0,chartLayoutContext/* selectChartLayout */.fz)(state);
    var xAxisId = (0,graphicalItemSelectors/* selectXAxisIdFromGraphicalItemId */.W)(state, id);
    var yAxisId = (0,graphicalItemSelectors/* selectYAxisIdFromGraphicalItemId */.B)(state, id);
    if (xAxisId == null || yAxisId == null) {
        return undefined;
    }
    if (layout === 'horizontal') {
        return (0,axisSelectors/* selectCartesianAxisSize */.BQ)(state, 'xAxis', xAxisId);
    }
    return (0,axisSelectors/* selectCartesianAxisSize */.BQ)(state, 'yAxis', yAxisId);
};
var selectBarSizeList = (0,reselect/* createSelector */.Mz)([
    selectAllVisibleBars,
    rootPropsSelectors/* selectRootBarSize */.x3,
    selectBarCartesianAxisSize
], combineBarSizeList);
var selectBarBandSize = (state, id, isPanorama)=>{
    var _ref, _getBandSizeOfAxis;
    var barSettings = selectSynchronisedBarSettings(state, id);
    if (barSettings == null) {
        return 0;
    }
    var xAxisId = (0,graphicalItemSelectors/* selectXAxisIdFromGraphicalItemId */.W)(state, id);
    var yAxisId = (0,graphicalItemSelectors/* selectYAxisIdFromGraphicalItemId */.B)(state, id);
    if (xAxisId == null || yAxisId == null) {
        return 0;
    }
    var layout = (0,chartLayoutContext/* selectChartLayout */.fz)(state);
    var globalMaxBarSize = (0,rootPropsSelectors/* selectRootMaxBarSize */.JN)(state);
    var { maxBarSize: childMaxBarSize } = barSettings;
    var maxBarSize = (0,DataUtils/* isNullish */.uy)(childMaxBarSize) ? globalMaxBarSize : childMaxBarSize;
    var axis, ticks;
    if (layout === 'horizontal') {
        axis = (0,axisSelectors/* selectAxisWithScale */.Gx)(state, 'xAxis', xAxisId, isPanorama);
        ticks = (0,axisSelectors/* selectTicksOfGraphicalItem */.CR)(state, 'xAxis', xAxisId, isPanorama);
    } else {
        axis = (0,axisSelectors/* selectAxisWithScale */.Gx)(state, 'yAxis', yAxisId, isPanorama);
        ticks = (0,axisSelectors/* selectTicksOfGraphicalItem */.CR)(state, 'yAxis', yAxisId, isPanorama);
    }
    return (_ref = (_getBandSizeOfAxis = (0,ChartUtils/* getBandSizeOfAxis */.Hj)(axis, ticks, true)) !== null && _getBandSizeOfAxis !== void 0 ? _getBandSizeOfAxis : maxBarSize) !== null && _ref !== void 0 ? _ref : 0;
};
var selectAxisBandSize = (state, id, isPanorama)=>{
    var layout = (0,chartLayoutContext/* selectChartLayout */.fz)(state);
    var xAxisId = (0,graphicalItemSelectors/* selectXAxisIdFromGraphicalItemId */.W)(state, id);
    var yAxisId = (0,graphicalItemSelectors/* selectYAxisIdFromGraphicalItemId */.B)(state, id);
    if (xAxisId == null || yAxisId == null) {
        return undefined;
    }
    var axis, ticks;
    if (layout === 'horizontal') {
        axis = (0,axisSelectors/* selectAxisWithScale */.Gx)(state, 'xAxis', xAxisId, isPanorama);
        ticks = (0,axisSelectors/* selectTicksOfGraphicalItem */.CR)(state, 'xAxis', xAxisId, isPanorama);
    } else {
        axis = (0,axisSelectors/* selectAxisWithScale */.Gx)(state, 'yAxis', yAxisId, isPanorama);
        ticks = (0,axisSelectors/* selectTicksOfGraphicalItem */.CR)(state, 'yAxis', yAxisId, isPanorama);
    }
    return (0,ChartUtils/* getBandSizeOfAxis */.Hj)(axis, ticks);
};
var selectAllBarPositions = (0,reselect/* createSelector */.Mz)([
    selectBarSizeList,
    rootPropsSelectors/* selectRootMaxBarSize */.JN,
    rootPropsSelectors/* selectBarGap */._5,
    rootPropsSelectors/* selectBarCategoryGap */.gY,
    selectBarBandSize,
    selectAxisBandSize,
    selectMaxBarSize
], combineAllBarPositions);
var selectXAxisWithScale = (state, id, isPanorama)=>{
    var xAxisId = (0,graphicalItemSelectors/* selectXAxisIdFromGraphicalItemId */.W)(state, id);
    if (xAxisId == null) {
        return undefined;
    }
    return (0,axisSelectors/* selectAxisWithScale */.Gx)(state, 'xAxis', xAxisId, isPanorama);
};
var selectYAxisWithScale = (state, id, isPanorama)=>{
    var yAxisId = (0,graphicalItemSelectors/* selectYAxisIdFromGraphicalItemId */.B)(state, id);
    if (yAxisId == null) {
        return undefined;
    }
    return (0,axisSelectors/* selectAxisWithScale */.Gx)(state, 'yAxis', yAxisId, isPanorama);
};
var selectXAxisTicks = (state, id, isPanorama)=>{
    var xAxisId = (0,graphicalItemSelectors/* selectXAxisIdFromGraphicalItemId */.W)(state, id);
    if (xAxisId == null) {
        return undefined;
    }
    return (0,axisSelectors/* selectTicksOfGraphicalItem */.CR)(state, 'xAxis', xAxisId, isPanorama);
};
var selectYAxisTicks = (state, id, isPanorama)=>{
    var yAxisId = (0,graphicalItemSelectors/* selectYAxisIdFromGraphicalItemId */.B)(state, id);
    if (yAxisId == null) {
        return undefined;
    }
    return (0,axisSelectors/* selectTicksOfGraphicalItem */.CR)(state, 'yAxis', yAxisId, isPanorama);
};
var selectBarPosition = (0,reselect/* createSelector */.Mz)([
    selectAllBarPositions,
    selectSynchronisedBarSettings
], combineBarPosition);
var selectStackedDataOfItem = (0,reselect/* createSelector */.Mz)([
    selectBarStackGroups,
    selectSynchronisedBarSettings
], combineStackedData);
var selectBarRectangles = (0,reselect/* createSelector */.Mz)([
    selectChartOffsetInternal/* selectChartOffsetInternal */.HZ,
    selectChartOffsetInternal/* selectAxisViewBox */.c2,
    selectXAxisWithScale,
    selectYAxisWithScale,
    selectXAxisTicks,
    selectYAxisTicks,
    selectBarPosition,
    chartLayoutContext/* selectChartLayout */.fz,
    dataSelectors/* selectChartDataWithIndexesIfNotInPanoramaPosition3 */.rN,
    selectAxisBandSize,
    selectStackedDataOfItem,
    selectSynchronisedBarSettings,
    pickCells
], (offset, axisViewBox, xAxis, yAxis, xAxisTicks, yAxisTicks, pos, layout, _ref2, bandSize, stackedData, barSettings, cells)=>{
    var { chartData, dataStartIndex, dataEndIndex } = _ref2;
    if (barSettings == null || pos == null || axisViewBox == null || layout !== 'horizontal' && layout !== 'vertical' || xAxis == null || yAxis == null || xAxisTicks == null || yAxisTicks == null || bandSize == null) {
        return undefined;
    }
    var { data } = barSettings;
    var displayedData;
    if (data != null && data.length > 0) {
        displayedData = data;
    } else {
        displayedData = chartData === null || chartData === void 0 ? void 0 : chartData.slice(dataStartIndex, dataEndIndex + 1);
    }
    if (displayedData == null) {
        return undefined;
    }
    return computeBarRectangles({
        layout,
        barSettings,
        pos,
        parentViewBox: axisViewBox,
        bandSize,
        xAxis,
        yAxis,
        xAxisTicks,
        yAxisTicks,
        stackedData,
        displayedData,
        offset,
        cells,
        dataStartIndex
    });
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/context/PanoramaContext.js
var PanoramaContext = __webpack_require__(73327);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/selectors/tooltipSelectors.js + 1 modules
var tooltipSelectors = __webpack_require__(930);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/SetLegendPayload.js
var SetLegendPayload = __webpack_require__(49190);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/useAnimationId.js
var useAnimationId = __webpack_require__(87810);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/resolveDefaultProps.js
var util_resolveDefaultProps = __webpack_require__(25055);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/context/RegisterGraphicalItemId.js + 2 modules
var RegisterGraphicalItemId = __webpack_require__(96228);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/state/SetGraphicalItem.js
var SetGraphicalItem = __webpack_require__(5021);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/svgPropertiesNoEvents.js
var svgPropertiesNoEvents = __webpack_require__(40959);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/animation/JavascriptAnimate.js + 6 modules
var JavascriptAnimate = __webpack_require__(70082);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/zIndex/ZIndexLayer.js
var ZIndexLayer = __webpack_require__(80731);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/zIndex/DefaultZIndexes.js
var DefaultZIndexes = __webpack_require__(71115);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/zIndex/getZIndexFromUnknown.js

function getZIndexFromUnknown(input, defaultZIndex) {
    if (input && typeof input === 'object' && 'zIndex' in input && typeof input.zIndex === 'number' && (0,isWellBehavedNumber/* isWellBehavedNumber */.H)(input.zIndex)) {
        return input.zIndex;
    }
    return defaultZIndex;
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/util/propsAreEqual.js
var util_propsAreEqual = __webpack_require__(7050);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/cartesian/BarStack.js
var _excluded = [
    "index"
];
function BarStack_extends() {
    return BarStack_extends = Object.assign ? Object.assign.bind() : function(n) {
        for(var e = 1; e < arguments.length; e++){
            var t = arguments[e];
            for(var r in t)({}).hasOwnProperty.call(t, r) && (n[r] = t[r]);
        }
        return n;
    }, BarStack_extends.apply(null, arguments);
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











var BarStackContext = /*#__PURE__*/ (0,react.createContext)(undefined);
/**
 * Hook to resolve the stack ID for a Bar component.
 * If a stack ID is provided via props, it is used directly.
 * Otherwise, this will read stack ID from BarStack context if available.
 * If both are undefined, it returns undefined.
 * @param childStackId
 */ var useStackId = (childStackId)=>{
    var stackSettings = (0,react.useContext)(BarStackContext);
    if (stackSettings != null) {
        return stackSettings.stackId;
    }
    if (childStackId == null) {
        return undefined;
    }
    return (0,ChartUtils/* getNormalizedStackId */.$8)(childStackId);
};
var defaultBarStackProps = {
    radius: 0
};
var getClipPathId = (stackId, index)=>{
    return "recharts-bar-stack-clip-path-".concat(stackId, "-").concat(index);
};
var useBarStackClipPathUrl = (index)=>{
    var barStackContext = (0,react.useContext)(BarStackContext);
    if (barStackContext == null) {
        return undefined;
    }
    var { stackId } = barStackContext;
    return "url(#".concat(getClipPathId(stackId, index), ")");
};
var BarStackClipLayer = (_ref)=>{
    var { index } = _ref, rest = _objectWithoutProperties(_ref, _excluded);
    var clipPathUrl = useBarStackClipPathUrl(index);
    return /*#__PURE__*/ react.createElement(Layer/* Layer */.W, BarStack_extends({
        className: "recharts-bar-stack-layer",
        clipPath: clipPathUrl
    }, rest));
};
/**
 * This React component will render a clipPath that the individual bars in the stack will reference
 * to achieve rounded corners for the entire stack.
 */ var BarStackClipPath = (_ref2)=>{
    var { stackId, radius } = _ref2;
    var isPanorama = useIsPanorama();
    var positions = useAppSelector((state)=>selectStackRects(state, stackId, isPanorama));
    if (positions == null || positions.length === 0) {
        return null;
    }
    /*
   * Render one clipPath per rectangle in the stack.
   * Each rectangle corresponds to one data entry in the chart.
   */ return /*#__PURE__*/ React.createElement("defs", null, positions.map((pos, index)=>{
        if (pos == null) {
            return null;
        }
        var clipPathId = getClipPathId(stackId, index);
        return /*#__PURE__*/ React.createElement("clipPath", {
            key: clipPathId,
            id: clipPathId
        }, /*#__PURE__*/ React.createElement(Rectangle, {
            isAnimationActive: false,
            isUpdateAnimationActive: false,
            x: pos.x,
            y: pos.y,
            width: pos.width,
            height: pos.height,
            radius: radius
        }));
    }));
};
var BarStackImpl = (props)=>{
    var resolvedStackId = useUniqueId('recharts-bar-stack', getNormalizedStackId(props.stackId));
    var { children, radius } = resolveDefaultProps(props, defaultBarStackProps);
    var context = useMemo(()=>({
            stackId: resolvedStackId,
            radius
        }), [
        resolvedStackId,
        radius
    ]);
    return /*#__PURE__*/ React.createElement(BarStackContext.Provider, {
        value: context
    }, /*#__PURE__*/ React.createElement(BarStackClipPath, {
        stackId: resolvedStackId,
        radius: radius
    }), children);
};
/**
 * @provides BarStackContext
 * @since 3.6
 */ var BarStack = /*#__PURE__*/ (/* unused pure expression or super */ null && (React.memo(BarStackImpl, propsAreEqual)));

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/recharts@3.8.1+ae7d356433e4ca31/node_modules/recharts/es6/cartesian/Bar.js
var Bar_excluded = [
    "onMouseEnter",
    "onMouseLeave",
    "onClick"
], _excluded2 = [
    "value",
    "background",
    "tooltipPosition"
], _excluded3 = [
    "id"
], _excluded4 = [
    "onMouseEnter",
    "onClick",
    "onMouseLeave"
];
function Bar_extends() {
    return Bar_extends = Object.assign ? Object.assign.bind() : function(n) {
        for(var e = 1; e < arguments.length; e++){
            var t = arguments[e];
            for(var r in t)({}).hasOwnProperty.call(t, r) && (n[r] = t[r]);
        }
        return n;
    }, Bar_extends.apply(null, arguments);
}
function Bar_ownKeys(e, r) {
    var t = Object.keys(e);
    if (Object.getOwnPropertySymbols) {
        var o = Object.getOwnPropertySymbols(e);
        r && (o = o.filter(function(r) {
            return Object.getOwnPropertyDescriptor(e, r).enumerable;
        })), t.push.apply(t, o);
    }
    return t;
}
function Bar_objectSpread(e) {
    for(var r = 1; r < arguments.length; r++){
        var t = null != arguments[r] ? arguments[r] : {};
        r % 2 ? Bar_ownKeys(Object(t), !0).forEach(function(r) {
            Bar_defineProperty(e, r, t[r]);
        }) : Object.getOwnPropertyDescriptors ? Object.defineProperties(e, Object.getOwnPropertyDescriptors(t)) : Bar_ownKeys(Object(t)).forEach(function(r) {
            Object.defineProperty(e, r, Object.getOwnPropertyDescriptor(t, r));
        });
    }
    return e;
}
function Bar_defineProperty(e, r, t) {
    return (r = Bar_toPropertyKey(r)) in e ? Object.defineProperty(e, r, {
        value: t,
        enumerable: !0,
        configurable: !0,
        writable: !0
    }) : e[r] = t, e;
}
function Bar_toPropertyKey(t) {
    var i = Bar_toPrimitive(t, "string");
    return "symbol" == typeof i ? i : i + "";
}
function Bar_toPrimitive(t, r) {
    if ("object" != typeof t || !t) return t;
    var e = t[Symbol.toPrimitive];
    if (void 0 !== e) {
        var i = e.call(t, r || "default");
        if ("object" != typeof i) return i;
        throw new TypeError("@@toPrimitive must return a primitive value.");
    }
    return ("string" === r ? String : Number)(t);
}
function Bar_objectWithoutProperties(e, t) {
    if (null == e) return {};
    var o, r, i = Bar_objectWithoutPropertiesLoose(e, t);
    if (Object.getOwnPropertySymbols) {
        var n = Object.getOwnPropertySymbols(e);
        for(r = 0; r < n.length; r++)o = n[r], -1 === t.indexOf(o) && ({}).propertyIsEnumerable.call(e, o) && (i[o] = e[o]);
    }
    return i;
}
function Bar_objectWithoutPropertiesLoose(r, e) {
    if (null == r) return {};
    var t = {};
    for(var n in r)if (({}).hasOwnProperty.call(r, n)) {
        if (-1 !== e.indexOf(n)) continue;
        t[n] = r[n];
    }
    return t;
}
































var computeLegendPayloadFromBarData = (props)=>{
    var { dataKey, name, fill, legendType, hide } = props;
    return [
        {
            inactive: hide,
            dataKey,
            type: legendType,
            color: fill,
            value: (0,ChartUtils/* getTooltipNameProp */.uM)(name, dataKey),
            payload: props
        }
    ];
};
var SetBarTooltipEntrySettings = /*#__PURE__*/ react.memo((_ref)=>{
    var { dataKey, stroke, strokeWidth, fill, name, hide, unit, tooltipType, id } = _ref;
    var tooltipEntrySettings = {
        dataDefinedOnItem: undefined,
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
            color: fill,
            unit,
            graphicalItemId: id
        }
    };
    return /*#__PURE__*/ react.createElement(SetTooltipEntrySettings/* SetTooltipEntrySettings */.r, {
        tooltipEntrySettings: tooltipEntrySettings
    });
});
function BarBackground(props) {
    var activeIndex = (0,hooks/* useAppSelector */.G)(tooltipSelectors/* selectActiveTooltipIndex */.A2);
    var { data, dataKey, background: backgroundFromProps, allOtherBarProps } = props;
    var { onMouseEnter: onMouseEnterFromProps, onMouseLeave: onMouseLeaveFromProps, onClick: onItemClickFromProps } = allOtherBarProps, restOfAllOtherProps = Bar_objectWithoutProperties(allOtherBarProps, Bar_excluded);
    var onMouseEnterFromContext = useMouseEnterItemDispatch(onMouseEnterFromProps, dataKey, allOtherBarProps.id);
    var onMouseLeaveFromContext = useMouseLeaveItemDispatch(onMouseLeaveFromProps);
    var onClickFromContext = useMouseClickItemDispatch(onItemClickFromProps, dataKey, allOtherBarProps.id);
    if (!backgroundFromProps || data == null) {
        return null;
    }
    var backgroundProps = (0,svgPropertiesNoEvents/* svgPropertiesNoEventsFromUnknown */.ic)(backgroundFromProps);
    return /*#__PURE__*/ react.createElement(ZIndexLayer/* ZIndexLayer */.g, {
        zIndex: getZIndexFromUnknown(backgroundFromProps, DefaultZIndexes/* DefaultZIndexes */.I.barBackground)
    }, data.map((entry, i)=>{
        var { value, background: backgroundFromDataEntry, tooltipPosition } = entry, rest = Bar_objectWithoutProperties(entry, _excluded2);
        if (!backgroundFromDataEntry) {
            return null;
        }
        var onMouseEnter = onMouseEnterFromContext(entry, i);
        var onMouseLeave = onMouseLeaveFromContext(entry, i);
        var onClick = onClickFromContext(entry, i);
        var barRectangleProps = Bar_objectSpread(Bar_objectSpread(Bar_objectSpread(Bar_objectSpread(Bar_objectSpread({
            option: backgroundFromProps,
            isActive: String(i) === activeIndex
        }, rest), {}, {
            // @ts-expect-error backgroundProps is contributing unknown props
            fill: '#eee'
        }, backgroundFromDataEntry), backgroundProps), (0,types/* adaptEventsOfChild */.XC)(restOfAllOtherProps, entry, i)), {}, {
            onMouseEnter,
            onMouseLeave,
            onClick,
            dataKey,
            index: i,
            className: 'recharts-bar-background-rectangle'
        });
        return /*#__PURE__*/ react.createElement(BarRectangle, Bar_extends({
            key: "background-bar-".concat(i)
        }, barRectangleProps));
    }));
}
function BarLabelListProvider(_ref2) {
    var { showLabels, children, rects } = _ref2;
    var labelListEntries = rects === null || rects === void 0 ? void 0 : rects.map((entry)=>{
        var viewBox = {
            x: entry.x,
            y: entry.y,
            width: entry.width,
            lowerWidth: entry.width,
            upperWidth: entry.width,
            height: entry.height
        };
        return Bar_objectSpread(Bar_objectSpread({}, viewBox), {}, {
            value: entry.value,
            payload: entry.payload,
            parentViewBox: entry.parentViewBox,
            viewBox,
            fill: entry.fill
        });
    });
    return /*#__PURE__*/ react.createElement(LabelList/* CartesianLabelListContextProvider */.h8, {
        value: showLabels ? labelListEntries : undefined
    }, children);
}
function BarRectangleWithActiveState(props) {
    var { shape, activeBar, baseProps, entry, index, dataKey } = props;
    var activeIndex = (0,hooks/* useAppSelector */.G)(tooltipSelectors/* selectActiveTooltipIndex */.A2);
    var activeDataKey = (0,hooks/* useAppSelector */.G)(tooltipSelectors/* selectActiveTooltipDataKey */.Xb);
    /*
   * Bars support stacking, meaning that there can be multiple bars at the same x value.
   * With Tooltip shared=false we only want to highlight the currently active Bar, not all.
   *
   * Also, if the tooltip is shared, we want to highlight all bars at the same x value
   * regardless of the dataKey.
   *
   * With shared Tooltip, the activeDataKey is undefined.
   *
   * We use entry.originalDataIndex to match against activeIndex because the render index parameter
   * is based on the filtered array, while activeIndex is based on the pre-filter displayed data slice.
   * When entries are filtered out (for example null/zero-dimension bars), these indices can differ.
   */ var isActive = activeBar && String(entry.originalDataIndex) === activeIndex && (activeDataKey == null || dataKey === activeDataKey);
    var [stayInLayer, setStayInLayer] = (0,react.useState)(false);
    var [hasMountedActive, setHasMountedActive] = (0,react.useState)(false);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        var rafId;
        if (isActive) {
            // 1. Enter the layer immediately
            setStayInLayer(true);
            // 2. Wait for the browser to paint the "inactive" state in the new layer,
            // then switch to active to trigger the CSS transition (width grow).
            rafId = requestAnimationFrame(()=>{
                setHasMountedActive(true);
            });
        } else {
            setHasMountedActive(false);
        }
        return ()=>{
            cancelAnimationFrame(rafId);
        };
    }, [
        isActive
    ]);
    var handleTransitionEnd = (0,react.useCallback)(()=>{
        // 4. Leave the layer only when the exit transition finishes
        if (!isActive) {
            setStayInLayer(false);
        }
    }, [
        isActive
    ]);
    // Determine props:
    // - If entering (isActive=true) but not mounted yet (hasMountedActive=false), pass isActive=false (inactive size).
    // - If exiting (isActive=false), pass isActive=false (inactive size).
    var isVisuallyActive = isActive && hasMountedActive;
    // Render in ZIndexLayer if active OR if we are waiting for exit transition
    var shouldRenderInLayer = isActive || stayInLayer;
    var option;
    if (isActive) {
        if (activeBar === true) {
            option = shape;
        } else {
            option = activeBar;
        }
    } else {
        option = shape;
    }
    var content = /*#__PURE__*/ react.createElement(BarRectangle, Bar_extends({}, baseProps, {
        name: String(baseProps.name)
    }, entry, {
        isActive: isVisuallyActive,
        option: option,
        index: index,
        dataKey: dataKey,
        onTransitionEnd: handleTransitionEnd
    }));
    if (shouldRenderInLayer) {
        return /*#__PURE__*/ react.createElement(ZIndexLayer/* ZIndexLayer */.g, {
            zIndex: DefaultZIndexes/* DefaultZIndexes */.I.activeBar
        }, /*#__PURE__*/ react.createElement(BarStackClipLayer, {
            index: entry.originalDataIndex
        }, content));
    }
    return content;
}
function BarRectangleNeverActive(props) {
    var { shape, baseProps, entry, index, dataKey } = props;
    return /*#__PURE__*/ react.createElement(BarRectangle, Bar_extends({}, baseProps, {
        name: String(baseProps.name)
    }, entry, {
        isActive: false,
        option: shape,
        index: index,
        dataKey: dataKey
    }));
}
function BarRectangles(_ref3) {
    var _svgPropertiesNoEvent;
    var { data, props } = _ref3;
    var _ref4 = (_svgPropertiesNoEvent = (0,svgPropertiesNoEvents/* svgPropertiesNoEvents */.uZ)(props)) !== null && _svgPropertiesNoEvent !== void 0 ? _svgPropertiesNoEvent : {}, { id } = _ref4, baseProps = Bar_objectWithoutProperties(_ref4, _excluded3);
    var { shape, dataKey, activeBar } = props;
    var { onMouseEnter: onMouseEnterFromProps, onClick: onItemClickFromProps, onMouseLeave: onMouseLeaveFromProps } = props, restOfAllOtherProps = Bar_objectWithoutProperties(props, _excluded4);
    var onMouseEnterFromContext = useMouseEnterItemDispatch(onMouseEnterFromProps, dataKey, id);
    var onMouseLeaveFromContext = useMouseLeaveItemDispatch(onMouseLeaveFromProps);
    var onClickFromContext = useMouseClickItemDispatch(onItemClickFromProps, dataKey, id);
    if (!data) {
        return null;
    }
    return /*#__PURE__*/ react.createElement(react.Fragment, null, data.map((entry, i)=>{
        return /*#__PURE__*/ react.createElement(BarStackClipLayer, Bar_extends({
            index: entry.originalDataIndex,
            key: "rectangle-".concat(entry === null || entry === void 0 ? void 0 : entry.x, "-").concat(entry === null || entry === void 0 ? void 0 : entry.y, "-").concat(entry === null || entry === void 0 ? void 0 : entry.value, "-").concat(i),
            className: "recharts-bar-rectangle"
        }, (0,types/* adaptEventsOfChild */.XC)(restOfAllOtherProps, entry, i), {
            onMouseEnter: onMouseEnterFromContext(entry, i),
            onMouseLeave: onMouseLeaveFromContext(entry, i),
            onClick: onClickFromContext(entry, i)
        }), activeBar ? /*#__PURE__*/ react.createElement(BarRectangleWithActiveState, {
            shape: shape,
            activeBar: activeBar,
            baseProps: baseProps,
            entry: entry,
            index: i,
            dataKey: dataKey
        }) : /*#__PURE__*/ /*
     * If the `activeBar` prop is falsy, then let's call the variant without hooks.
     * Using the `selectActiveTooltipIndex` selector is usually fast
     * but in charts with large-ish amount of data even the few nanoseconds add up to a noticeable jank.
     * If the activeBar is false then we don't need to know which index is active - because we won't use it anyway.
     * So let's just skip the hooks altogether. That way, React can skip rendering the component,
     * and can skip the tree reconciliation for its children too.
     * Because we can't call hooks conditionally, we need to have a separate component for that.
     */ react.createElement(BarRectangleNeverActive, {
            shape: shape,
            baseProps: baseProps,
            entry: entry,
            index: i,
            dataKey: dataKey
        }));
    }));
}
function RectanglesWithAnimation(_ref5) {
    var { props, previousRectanglesRef } = _ref5;
    var { data, layout, isAnimationActive, animationBegin, animationDuration, animationEasing, onAnimationEnd, onAnimationStart } = props;
    var prevData = previousRectanglesRef.current;
    var animationId = (0,useAnimationId/* useAnimationId */.n)(props, 'recharts-bar-');
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
    return /*#__PURE__*/ react.createElement(BarLabelListProvider, {
        showLabels: showLabels,
        rects: data
    }, /*#__PURE__*/ react.createElement(JavascriptAnimate/* JavascriptAnimate */.J, {
        animationId: animationId,
        begin: animationBegin,
        duration: animationDuration,
        isActive: isAnimationActive,
        easing: animationEasing,
        onAnimationEnd: handleAnimationEnd,
        onAnimationStart: handleAnimationStart,
        key: animationId
    }, (t)=>{
        var stepData = t === 1 ? data : data === null || data === void 0 ? void 0 : data.map((entry, index)=>{
            var prev = prevData && prevData[index];
            if (prev) {
                return Bar_objectSpread(Bar_objectSpread({}, entry), {}, {
                    x: (0,DataUtils/* interpolate */.GW)(prev.x, entry.x, t),
                    y: (0,DataUtils/* interpolate */.GW)(prev.y, entry.y, t),
                    width: (0,DataUtils/* interpolate */.GW)(prev.width, entry.width, t),
                    height: (0,DataUtils/* interpolate */.GW)(prev.height, entry.height, t)
                });
            }
            if (layout === 'horizontal') {
                var height = (0,DataUtils/* interpolate */.GW)(0, entry.height, t);
                var y = (0,DataUtils/* interpolate */.GW)(entry.stackedBarStart, entry.y, t);
                return Bar_objectSpread(Bar_objectSpread({}, entry), {}, {
                    y,
                    height
                });
            }
            var w = (0,DataUtils/* interpolate */.GW)(0, entry.width, t);
            var x = (0,DataUtils/* interpolate */.GW)(entry.stackedBarStart, entry.x, t);
            return Bar_objectSpread(Bar_objectSpread({}, entry), {}, {
                width: w,
                x
            });
        });
        if (t > 0) {
            // eslint-disable-next-line no-param-reassign
            previousRectanglesRef.current = stepData !== null && stepData !== void 0 ? stepData : null;
        }
        if (stepData == null) {
            return null;
        }
        return /*#__PURE__*/ react.createElement(Layer/* Layer */.W, null, /*#__PURE__*/ react.createElement(BarRectangles, {
            props: props,
            data: stepData
        }));
    }), /*#__PURE__*/ react.createElement(LabelList/* LabelListFromLabelProp */.qY, {
        label: props.label
    }), props.children);
}
function RenderRectangles(props) {
    var previousRectanglesRef = (0,react.useRef)(null);
    return /*#__PURE__*/ react.createElement(RectanglesWithAnimation, {
        previousRectanglesRef: previousRectanglesRef,
        props: props
    });
}
var defaultMinPointSize = 0;
var errorBarDataPointFormatter = (dataPoint, dataKey)=>{
    /**
   * if the value coming from `selectBarRectangles` is an array then this is a stacked bar chart.
   * arr[1] represents end value of the bar since the data is in the form of [startValue, endValue].
   * */ var value = Array.isArray(dataPoint.value) ? dataPoint.value[1] : dataPoint.value;
    return {
        x: dataPoint.x,
        y: dataPoint.y,
        value,
        // getValueByDataKey does not validate the output type
        errorVal: (0,ChartUtils/* getValueByDataKey */.kr)(dataPoint, dataKey)
    };
};
class BarWithState extends react.PureComponent {
    render() {
        var { hide, data, dataKey, className, xAxisId, yAxisId, needClip, background, id } = this.props;
        if (hide || data == null) {
            return null;
        }
        var layerClass = (0,clsx/* clsx */.$)('recharts-bar', className);
        var clipPathId = id;
        return /*#__PURE__*/ react.createElement(Layer/* Layer */.W, {
            className: layerClass,
            id: id
        }, needClip && /*#__PURE__*/ react.createElement("defs", null, /*#__PURE__*/ react.createElement(GraphicalItemClipPath/* GraphicalItemClipPath */.Q, {
            clipPathId: clipPathId,
            xAxisId: xAxisId,
            yAxisId: yAxisId
        })), /*#__PURE__*/ react.createElement(Layer/* Layer */.W, {
            className: "recharts-bar-rectangles",
            clipPath: needClip ? "url(#clipPath-".concat(clipPathId, ")") : undefined
        }, /*#__PURE__*/ react.createElement(BarBackground, {
            data: data,
            dataKey: dataKey,
            background: background,
            allOtherBarProps: this.props
        }), /*#__PURE__*/ react.createElement(RenderRectangles, this.props)));
    }
}
var defaultBarProps = {
    activeBar: false,
    animationBegin: 0,
    animationDuration: 400,
    animationEasing: 'ease',
    background: false,
    hide: false,
    isAnimationActive: 'auto',
    label: false,
    legendType: 'rect',
    minPointSize: defaultMinPointSize,
    xAxisId: 0,
    yAxisId: 0,
    zIndex: DefaultZIndexes/* DefaultZIndexes */.I.bar
};
function BarImpl(props) {
    var { xAxisId, yAxisId, hide, legendType, minPointSize, activeBar, animationBegin, animationDuration, animationEasing, isAnimationActive } = props;
    var { needClip } = (0,GraphicalItemClipPath/* useNeedsClip */.l)(xAxisId, yAxisId);
    var layout = (0,chartLayoutContext/* useChartLayout */.WX)();
    var isPanorama = (0,PanoramaContext/* useIsPanorama */.r)();
    var cells = (0,ReactUtils/* findAllByType */.aS)(props.children, Cell/* Cell */.f);
    var rects = (0,hooks/* useAppSelector */.G)((state)=>selectBarRectangles(state, props.id, isPanorama, cells));
    if (layout !== 'vertical' && layout !== 'horizontal') {
        return null;
    }
    var errorBarOffset;
    var firstDataPoint = rects === null || rects === void 0 ? void 0 : rects[0];
    if (firstDataPoint == null || firstDataPoint.height == null || firstDataPoint.width == null) {
        errorBarOffset = 0;
    } else {
        errorBarOffset = layout === 'vertical' ? firstDataPoint.height / 2 : firstDataPoint.width / 2;
    }
    return /*#__PURE__*/ react.createElement(ErrorBarContext/* SetErrorBarContext */.zk, {
        xAxisId: xAxisId,
        yAxisId: yAxisId,
        data: rects,
        dataPointFormatter: errorBarDataPointFormatter,
        errorBarOffset: errorBarOffset
    }, /*#__PURE__*/ react.createElement(BarWithState, Bar_extends({}, props, {
        layout: layout,
        needClip: needClip,
        data: rects,
        xAxisId: xAxisId,
        yAxisId: yAxisId,
        hide: hide,
        legendType: legendType,
        minPointSize: minPointSize,
        activeBar: activeBar,
        animationBegin: animationBegin,
        animationDuration: animationDuration,
        animationEasing: animationEasing,
        isAnimationActive: isAnimationActive
    })));
}
function computeBarRectangles(_ref6) {
    var { layout, barSettings: { dataKey, minPointSize: minPointSizeProp, hasCustomShape }, pos, bandSize, xAxis, yAxis, xAxisTicks, yAxisTicks, stackedData, displayedData, offset, cells, parentViewBox, dataStartIndex } = _ref6;
    var numericAxis = layout === 'horizontal' ? yAxis : xAxis;
    // @ts-expect-error this assumes that the domain is always numeric, but doesn't check for it
    var stackedDomain = stackedData ? numericAxis.scale.domain() : null;
    var baseValue = (0,ChartUtils/* getBaseValueOfBar */.DW)({
        numericAxis
    });
    var stackedBarStart = numericAxis.scale.map(baseValue);
    return displayedData.map((entry, index)=>{
        var value, x, y, width, height, background;
        if (stackedData) {
            // Use dataStartIndex to access the correct element in the full stackedData array
            var untruncatedValue = stackedData[index + dataStartIndex];
            if (untruncatedValue == null) {
                return null;
            }
            value = (0,ChartUtils/* truncateByDomain */._f)(untruncatedValue, stackedDomain);
        } else {
            value = (0,ChartUtils/* getValueByDataKey */.kr)(entry, dataKey);
            if (!Array.isArray(value)) {
                value = [
                    baseValue,
                    value
                ];
            }
        }
        var minPointSize = minPointSizeCallback(minPointSizeProp, defaultMinPointSize)(value[1], index);
        if (layout === 'horizontal') {
            var _ref7;
            var baseValueScale = yAxis.scale.map(value[0]);
            var currentValueScale = yAxis.scale.map(value[1]);
            if (baseValueScale == null || currentValueScale == null) {
                return null;
            }
            x = (0,ChartUtils/* getCateCoordinateOfBar */.y2)({
                axis: xAxis,
                ticks: xAxisTicks,
                bandSize,
                offset: pos.offset,
                entry,
                index
            });
            y = (_ref7 = currentValueScale !== null && currentValueScale !== void 0 ? currentValueScale : baseValueScale) !== null && _ref7 !== void 0 ? _ref7 : undefined;
            width = pos.size;
            var computedHeight = baseValueScale - currentValueScale;
            height = (0,DataUtils/* isNan */.M8)(computedHeight) ? 0 : computedHeight;
            background = {
                x,
                y: offset.top,
                width,
                height: offset.height
            };
            if (Math.abs(minPointSize) > 0 && Math.abs(height) < Math.abs(minPointSize)) {
                var delta = (0,DataUtils/* mathSign */.sA)(height || minPointSize) * (Math.abs(minPointSize) - Math.abs(height));
                y -= delta;
                height += delta;
            }
        } else {
            var _baseValueScale = xAxis.scale.map(value[0]);
            var _currentValueScale = xAxis.scale.map(value[1]);
            if (_baseValueScale == null || _currentValueScale == null) {
                return null;
            }
            x = _baseValueScale;
            y = (0,ChartUtils/* getCateCoordinateOfBar */.y2)({
                axis: yAxis,
                ticks: yAxisTicks,
                bandSize,
                offset: pos.offset,
                entry,
                index
            });
            width = _currentValueScale - _baseValueScale;
            height = pos.size;
            background = {
                x: offset.left,
                y,
                width: offset.width,
                height
            };
            if (Math.abs(minPointSize) > 0 && Math.abs(width) < Math.abs(minPointSize)) {
                var _delta = (0,DataUtils/* mathSign */.sA)(width || minPointSize) * (Math.abs(minPointSize) - Math.abs(width));
                width += _delta;
            }
        }
        /*
     * Filter out 0-dimension rectangles early to avoid creating unnecessary component trees.
     * BarStack clip-paths use originalDataIndex, so sparse filtered arrays remain index-stable.
     * Bars with a custom shape are not filtered out: the custom renderer may still draw something
     * visible at zero-dimension positions (e.g. horizontal lines in a BoxPlot).
     */ if (x == null || y == null || width == null || height == null || !hasCustomShape && (width === 0 || height === 0)) {
            return null;
        }
        var barRectangleItem = Bar_objectSpread(Bar_objectSpread({}, entry), {}, {
            stackedBarStart,
            x,
            y,
            width,
            height,
            value: stackedData ? value : value[1],
            payload: entry,
            background,
            tooltipPosition: {
                x: x + width / 2,
                y: y + height / 2
            },
            parentViewBox,
            originalDataIndex: index
        }, cells && cells[index] && cells[index].props);
        return barRectangleItem;
    }).filter(Boolean);
}
function BarFn(outsideProps) {
    var props = (0,util_resolveDefaultProps/* resolveDefaultProps */.e)(outsideProps, defaultBarProps);
    // stackId may arrive from props or from BarStack context
    var stackId = useStackId(props.stackId);
    var isPanorama = (0,PanoramaContext/* useIsPanorama */.r)();
    // Report all props to Redux store first, before calling any hooks, to avoid circular dependencies.
    return /*#__PURE__*/ react.createElement(RegisterGraphicalItemId/* RegisterGraphicalItemId */.x, {
        id: props.id,
        type: "bar"
    }, (id)=>/*#__PURE__*/ react.createElement(react.Fragment, null, /*#__PURE__*/ react.createElement(SetLegendPayload/* SetLegendPayload */.A, {
            legendPayload: computeLegendPayloadFromBarData(props)
        }), /*#__PURE__*/ react.createElement(SetBarTooltipEntrySettings, {
            dataKey: props.dataKey,
            stroke: props.stroke,
            strokeWidth: props.strokeWidth,
            fill: props.fill,
            name: props.name,
            hide: props.hide,
            unit: props.unit,
            tooltipType: props.tooltipType,
            id: id
        }), /*#__PURE__*/ react.createElement(SetGraphicalItem/* SetCartesianGraphicalItem */.p, {
            type: "bar",
            id: id,
            data: undefined,
            xAxisId: props.xAxisId,
            yAxisId: props.yAxisId,
            zAxisId: 0,
            dataKey: props.dataKey,
            stackId: stackId,
            hide: props.hide,
            barSize: props.barSize,
            minPointSize: props.minPointSize,
            maxBarSize: props.maxBarSize,
            isPanorama: isPanorama,
            hasCustomShape: props.shape != null
        }), /*#__PURE__*/ react.createElement(ZIndexLayer/* ZIndexLayer */.g, {
            zIndex: props.zIndex
        }, /*#__PURE__*/ react.createElement(BarImpl, Bar_extends({}, props, {
            id: id
        })))));
}
/**
 * @provides ErrorBarContext
 * @provides LabelListContext
 * @provides CellReader
 * @consumes CartesianChartContext
 * @consumes BarStackContext
 */ var Bar = /*#__PURE__*/ react.memo(BarFn, util_propsAreEqual/* propsAreEqual */.P);
// @ts-expect-error we need to set the displayName for debugging purposes
Bar.displayName = 'Bar';


/***/ }),

/***/ 45951:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   E: () => (/* binding */ BarChart)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var _state_optionsSlice__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(81295);
/* harmony import */ var _CartesianChart__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(20141);




var allowedTooltipTypes = [
    'axis',
    'item'
];
/**
 * @consumes ResponsiveContainerContext
 * @provides CartesianViewBoxContext
 * @provides CartesianChartContext
 */ var BarChart = /*#__PURE__*/ (0,react__WEBPACK_IMPORTED_MODULE_0__.forwardRef)((props, ref)=>{
    return /*#__PURE__*/ react__WEBPACK_IMPORTED_MODULE_0__.createElement(_CartesianChart__WEBPACK_IMPORTED_MODULE_1__/* .CartesianChart */ .P, {
        chartName: "BarChart",
        defaultTooltipEventType: "axis",
        validateTooltipEventTypes: allowedTooltipTypes,
        tooltipPayloadSearcher: _state_optionsSlice__WEBPACK_IMPORTED_MODULE_2__/* .arrayTooltipSearcher */ .uN,
        categoricalChartProps: props,
        ref: ref
    });
});


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

/***/ 68213:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   f: () => (/* binding */ Cell)
/* harmony export */ });
/**
 * Cell component used to define colors and styles of chart elements.
 *
 * This component is now deprecated and will be removed in Recharts 4.0.
 *
 * Please use the `shape` prop or `content` prop on the respective chart components
 * to customize the rendering of chart elements instead of using `Cell`.
 *
 * @see {@link https://recharts.github.io/en-US/guide/cell/ Guide: Migrate from Cell component to shape prop}
 *
 * @deprecated
 * @consumes CellReader
 */ var Cell = (_props)=>null;
Cell.displayName = 'Cell';


/***/ })

};
;