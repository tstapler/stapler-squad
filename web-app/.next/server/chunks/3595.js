"use strict";
exports.id = 3595;
exports.ids = [3595];
exports.modules = {

/***/ 7446:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {



if (true) {
  module.exports = __webpack_require__(27955);
} else {}


/***/ }),

/***/ 23810:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".kqw3cc0 {\n  background: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-left: 3px solid var(--_19ez2rzi);\n  border-radius: var(--_19ez2rz38);\n  padding: var(--_19ez2rz31);\n}\n.kqw3cc1 {\n  margin: 0;\n  margin-bottom: var(--_19ez2rz30);\n  font-size: var(--_19ez2rz3c);\n  font-weight: 700;\n  color: var(--_19ez2rz0);\n}\n.kqw3cc2 {\n  color: var(--_19ez2rz12);\n  font-size: var(--_19ez2rz3b);\n  margin-bottom: var(--_19ez2rz2z);\n}\n.kqw3cc3 {\n  list-style: none;\n  margin: 0;\n  padding: 0;\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz30);\n}\n.kqw3cc4 {\n  border-top: 1px solid var(--_19ez2rze);\n  padding-top: var(--_19ez2rz30);\n}\n.kqw3cc4:first-child {\n  border-top: none;\n  padding-top: 0;\n}\n.kqw3cc5 {\n  display: flex;\n  align-items: baseline;\n  gap: var(--_19ez2rz2z);\n  margin-bottom: var(--_19ez2rz2z);\n}\n.kqw3cc6 {\n  font-size: var(--_19ez2rz3a);\n  font-weight: 600;\n  padding: 2px var(--_19ez2rz2z);\n  border-radius: var(--_19ez2rz36);\n  white-space: nowrap;\n  flex-shrink: 0;\n  background: var(--_19ez2rzy);\n  color: var(--_19ez2rzz);\n  border: 1px solid var(--_19ez2rzx);\n}\n.kqw3cc7 {\n  font-size: var(--_19ez2rz3a);\n  font-weight: 600;\n  padding: 2px var(--_19ez2rz2z);\n  border-radius: var(--_19ez2rz36);\n  white-space: nowrap;\n  flex-shrink: 0;\n  background: var(--_19ez2rzv);\n  color: var(--_19ez2rzw);\n  border: 1px solid var(--_19ez2rzu);\n}\n.kqw3cc8 {\n  margin: 0;\n  font-size: var(--_19ez2rz3c);\n  color: var(--_19ez2rz0);\n}\n.kqw3cc9 {\n  margin: 0;\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz1);\n}\n.kqw3cca {\n  display: flex;\n  flex-wrap: wrap;\n  gap: var(--_19ez2rz2z);\n}\n.kqw3ccb {\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n  background: var(--_19ez2rzc);\n  color: var(--_19ez2rz0);\n  font-size: var(--_19ez2rz3b);\n  font-weight: 600;\n  cursor: pointer;\n  transition: all 0.15s ease;\n}\n.kqw3ccb:hover:not(:disabled) {\n  background: var(--_19ez2rze);\n}\n.kqw3ccb:disabled {\n  opacity: 0.6;\n  cursor: not-allowed;\n}\n.kqw3ccc {\n  display: flex;\n  gap: var(--_19ez2rz2z);\n}\n.kqw3ccd {\n  flex: 1 1;\n  padding: var(--_19ez2rz2z) var(--_19ez2rz30);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n  background: var(--_19ez2rz6);\n  color: var(--_19ez2rz0);\n  font-size: var(--_19ez2rz3b);\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 27955:
/***/ ((__unused_webpack_module, exports, __webpack_require__) => {

/**
 * @license React
 * use-sync-external-store-shim.production.js
 *
 * Copyright (c) Meta Platforms, Inc. and affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */


var React = __webpack_require__(88782);
function is(x, y) {
  return (x === y && (0 !== x || 1 / x === 1 / y)) || (x !== x && y !== y);
}
var objectIs = "function" === typeof Object.is ? Object.is : is,
  useState = React.useState,
  useEffect = React.useEffect,
  useLayoutEffect = React.useLayoutEffect,
  useDebugValue = React.useDebugValue;
function useSyncExternalStore$2(subscribe, getSnapshot) {
  var value = getSnapshot(),
    _useState = useState({ inst: { value: value, getSnapshot: getSnapshot } }),
    inst = _useState[0].inst,
    forceUpdate = _useState[1];
  useLayoutEffect(
    function () {
      inst.value = value;
      inst.getSnapshot = getSnapshot;
      checkIfSnapshotChanged(inst) && forceUpdate({ inst: inst });
    },
    [subscribe, value, getSnapshot]
  );
  useEffect(
    function () {
      checkIfSnapshotChanged(inst) && forceUpdate({ inst: inst });
      return subscribe(function () {
        checkIfSnapshotChanged(inst) && forceUpdate({ inst: inst });
      });
    },
    [subscribe]
  );
  useDebugValue(value);
  return value;
}
function checkIfSnapshotChanged(inst) {
  var latestGetSnapshot = inst.getSnapshot;
  inst = inst.value;
  try {
    var nextValue = latestGetSnapshot();
    return !objectIs(inst, nextValue);
  } catch (error) {
    return !0;
  }
}
function useSyncExternalStore$1(subscribe, getSnapshot) {
  return getSnapshot();
}
var shim =
  "undefined" === typeof window ||
  "undefined" === typeof window.document ||
  "undefined" === typeof window.document.createElement
    ? useSyncExternalStore$1
    : useSyncExternalStore$2;
exports.useSyncExternalStore =
  void 0 !== React.useSyncExternalStore ? React.useSyncExternalStore : shim;


/***/ }),

/***/ 42934:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._13flcz0 {\n  display: flex;\n  flex-direction: column;\n}\n._13flcz1 {\n  display: flex;\n  align-items: center;\n  gap: 4px;\n  padding: 0 8px;\n  font-size: 13px;\n  background: var(--_19ez2rzy);\n  color: var(--_19ez2rzz);\n  border-bottom: 1px solid var(--_19ez2rzx);\n}\n._13flcz2 {\n  flex: 1 1;\n  min-width: 0;\n  overflow: hidden;\n  text-overflow: ellipsis;\n  white-space: nowrap;\n}\n._13flcz3 {\n  align-self: flex-start;\n  border: none;\n  background: transparent;\n  color: var(--_19ez2rz19);\n  font: inherit;\n  font-size: 12px;\n  text-decoration: underline;\n  cursor: pointer;\n  padding: 2px 10px;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 43788:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "@keyframes z78t1g2 {\n  from {\n    transform: rotate(0deg);\n  }\n  to {\n    transform: rotate(360deg);\n  }\n}\n.z78t1g0 {\n  display: inline-flex;\n  align-items: center;\n  gap: 6px;\n  font-size: 0.75rem;\n  font-weight: 600;\n  -webkit-user-select: none;\n     -moz-user-select: none;\n          user-select: none;\n  white-space: nowrap;\n}\n.z78t1g1 {\n  width: 8px;\n  height: 8px;\n  border-radius: 50%;\n  flex-shrink: 0;\n  display: inline-block;\n}\n.z78t1g3 {\n  width: 8px;\n  height: 8px;\n  border-radius: 50%;\n  flex-shrink: 0;\n  display: inline-block;\n  border: 2px solid var(--_19ez2rzx);\n  border-top-color: transparent;\n}\n.z78t1g4 {\n  background: var(--_19ez2rzu);\n}\n.z78t1g7 {\n  background: var(--_19ez2rzx);\n  border: 1px solid var(--_19ez2rzz);\n}\n.z78t1g8 {\n  background: var(--_19ez2rz2);\n}\n.z78t1g9 {\n  color: var(--_19ez2rz1);\n}\n@media (prefers-reduced-motion: no-preference) {\n  .z78t1g3 {\n    animation-name: z78t1g2;\n    animation-duration: 0.8s;\n    animation-timing-function: linear;\n    animation-iteration-count: infinite;\n  }\n}\n@media (prefers-reduced-motion: reduce) {\n  .z78t1g3 {\n    border-top-color: var(--_19ez2rzx);\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 44632:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "@keyframes zl9j530 {\n  to {\n    transform: rotate(360deg);\n  }\n}\n@keyframes zl9j53s {\n  0% {\n    background-position: 200% 0;\n  }\n  100% {\n    background-position: -200% 0;\n  }\n}\n.zl9j531 {\n  display: flex;\n  flex-direction: column;\n  height: 100%;\n  overflow: hidden;\n  background: var(--_19ez2rz1d);\n}\n.zl9j532 {\n  display: flex;\n  flex-direction: column;\n  align-items: center;\n  justify-content: center;\n  height: 100%;\n  color: var(--_19ez2rz2);\n  gap: 12px;\n}\n.zl9j533 {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz1);\n  margin: 0;\n}\n.zl9j534 {\n  font-size: 48px;\n  opacity: 0.5;\n}\n.zl9j535 {\n  display: flex;\n  align-items: center;\n  gap: 8px;\n  padding: 24px;\n  color: var(--_19ez2rz2);\n}\n.zl9j536 {\n  padding: 24px;\n  color: var(--_19ez2rz10);\n}\n.zl9j537 {\n  display: inline-block;\n  width: 14px;\n  height: 14px;\n  border: 2px solid var(--_19ez2rze);\n  border-top-color: var(--_19ez2rzm);\n  border-radius: 50%;\n  animation: zl9j530 0.6s linear infinite;\n}\n.zl9j538 {\n  display: flex;\n  align-items: center;\n  flex-wrap: wrap;\n  gap: 0;\n  padding: 6px 12px;\n  border-bottom: 1px solid var(--_19ez2rze);\n  font-size: 12px;\n  font-family: var(--_19ez2rz2u);\n  background: var(--_19ez2rz1i);\n  overflow: hidden;\n}\n.zl9j539 {\n  color: var(--_19ez2rz2);\n  background: none;\n  border: none;\n  cursor: pointer;\n  padding: 0;\n  font: inherit;\n}\n.zl9j53a {\n  color: var(--_19ez2rz1e);\n  font-weight: 500;\n}\n.zl9j53b {\n  color: var(--_19ez2rz2);\n  padding: 0 2px;\n}\n.zl9j53c {\n  padding: 6px 12px;\n  background: var(--_19ez2rzy);\n  border-bottom: 1px solid var(--_19ez2rzx);\n  color: var(--_19ez2rzx);\n  font-size: 12px;\n}\n.zl9j53d {\n  flex: 1 1;\n  overflow: auto;\n  height: 0;\n}\n.zl9j53e {\n  display: inline-flex;\n  align-items: center;\n  padding: 2px var(--_19ez2rz2z);\n  font-size: var(--_19ez2rz3b);\n  border-radius: var(--_19ez2rz36);\n  color: var(--_19ez2rz1);\n  border: 1px solid var(--_19ez2rze);\n  background: transparent;\n  cursor: pointer;\n}\n.zl9j53e:hover {\n  color: var(--_19ez2rz0);\n  background-color: var(--_19ez2rz8);\n}\n.zl9j53f {\n  color: var(--_19ez2rzm);\n  border-color: var(--_19ez2rzm);\n}\n.zl9j53g {\n  display: flex;\n  flex-direction: column;\n  align-items: center;\n  justify-content: center;\n  height: 100%;\n  gap: 8px;\n  color: var(--_19ez2rz2);\n}\n.zl9j53h {\n  font-size: 48px;\n  opacity: 0.5;\n}\n.zl9j53i {\n  font-size: 15px;\n  color: var(--_19ez2rz1e);\n  margin: 0;\n}\n.zl9j53j {\n  font-size: 12px;\n  margin: 0;\n}\n.zl9j53k {\n  display: inline-flex;\n  align-items: center;\n  gap: var(--_19ez2rz2y);\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  font-size: var(--_19ez2rz3b);\n  border-radius: var(--_19ez2rz36);\n  text-decoration: none;\n  color: var(--_19ez2rz1);\n  border: 1px solid var(--_19ez2rze);\n  margin-left: auto;\n}\n.zl9j53k:hover {\n  color: var(--_19ez2rz0);\n  background-color: var(--_19ez2rz8);\n}\n.zl9j53l {\n  display: flex;\n  align-items: center;\n  justify-content: center;\n  flex: 1 1;\n  overflow: auto;\n  padding: var(--_19ez2rz31);\n}\n.zl9j53m {\n  max-width: 100%;\n  max-height: 100%;\n  object-fit: contain;\n}\n.zl9j53n {\n  flex: 1 1;\n  height: 0;\n  overflow: hidden;\n}\n.zl9j53o {\n  width: 100%;\n  height: 100%;\n  border: none;\n  display: block;\n}\n.zl9j53p {\n  display: flex;\n  flex-direction: column;\n  align-items: center;\n  justify-content: center;\n  flex: 1 1;\n  overflow: auto;\n  padding: var(--_19ez2rz31);\n  gap: var(--_19ez2rz30);\n}\n.zl9j53q {\n  max-width: 100%;\n  max-height: calc(100% - 48px);\n}\n.zl9j53r {\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz2);\n  text-align: center;\n  margin: 0;\n}\n@media (pointer: coarse) {\n  .zl9j539 {\n    min-height: 44px;\n    min-width: 44px;\n  }\n  .zl9j53e {\n    min-height: 44px;\n    min-width: 44px;\n    justify-content: center;\n  }\n  .zl9j53k {\n    min-height: 44px;\n    min-width: 44px;\n    justify-content: center;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 52298:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".gfgef0 {\n  color: var(--_19ez2rz0);\n  font-size: var(--_19ez2rz3b);\n  line-height: 1.6;\n}\n.gfgef0 p {\n  margin: 0 0 var(--_19ez2rz2z) 0;\n}\n.gfgef0 p:last-child {\n  margin-bottom: 0;\n}\n.gfgef0 a {\n  color: var(--_19ez2rzm);\n  text-decoration: underline;\n}\n.gfgef0 img {\n  max-width: 100%;\n  border-radius: var(--_19ez2rz37);\n  border: 1px solid var(--_19ez2rzf);\n}\n.gfgef0 code {\n  font-family: var(--_19ez2rz2u);\n  font-size: 0.875em;\n  background: var(--_19ez2rz7);\n  padding: 0.15em 0.35em;\n  border-radius: var(--_19ez2rz36);\n  border: 1px solid var(--_19ez2rzf);\n}\n.gfgef0 pre {\n  background: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n  padding: var(--_19ez2rz30);\n  overflow-x: auto;\n}\n.gfgef0 pre code {\n  background: transparent;\n  border: none;\n  padding: 0;\n}\n.gfgef0 ul, .gfgef0 ol {\n  padding-left: var(--_19ez2rz32);\n  margin-bottom: var(--_19ez2rz2z);\n}\n.gfgef0 blockquote {\n  border-left: 3px solid var(--_19ez2rzm);\n  padding-left: var(--_19ez2rz30);\n  color: var(--_19ez2rz2);\n  margin-left: 0;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 55280:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "@keyframes _17q7em30 {\n  to {\n    transform: rotate(360deg);\n  }\n}\n._17q7em31 {\n  width: 100%;\n  height: 100%;\n  overflow: hidden;\n  background: var(--_19ez2rz1d);\n  font-family: var(--_19ez2rz2u);\n  font-size: 13px;\n}\n._17q7em32 {\n  display: flex;\n  align-items: center;\n  gap: 8px;\n  padding: 16px;\n  color: var(--_19ez2rz2);\n}\n._17q7em33 {\n  display: flex;\n  flex-direction: column;\n  align-items: flex-start;\n  gap: 8px;\n  padding: 16px;\n  color: var(--_19ez2rz10);\n}\n._17q7em34 {\n  padding: 4px 10px;\n  font-size: 12px;\n  background: var(--_19ez2rz1d);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: 4px;\n  color: var(--_19ez2rz1e);\n  cursor: pointer;\n}\n._17q7em34:hover {\n  background: var(--_19ez2rz1k);\n}\n._17q7em35 {\n  padding: 16px;\n  color: var(--_19ez2rz2);\n  font-style: italic;\n}\n._17q7em36 {\n  display: flex;\n  align-items: center;\n  height: 28px;\n  cursor: pointer;\n  -webkit-user-select: none;\n     -moz-user-select: none;\n          user-select: none;\n  border-radius: 3px;\n}\n._17q7em36:hover {\n  background: var(--_19ez2rz1k);\n}\n._17q7em37 {\n  background: var(--selection-bg, rgba(40, 100, 255, 0.25)) !important;\n}\n._17q7em38 {\n  outline: 2px solid var(--_19ez2rzm);\n  outline-offset: -2px;\n}\n._17q7em39 {\n  display: flex;\n  align-items: center;\n  gap: 5px;\n  width: 100%;\n  padding-right: 8px;\n  overflow: hidden;\n}\n._17q7em3a {\n  flex-shrink: 0;\n  width: 16px;\n  text-align: center;\n  font-size: 11px;\n  color: var(--_19ez2rz2);\n}\n._17q7em3b {\n  flex: 1 1;\n  overflow: hidden;\n  text-overflow: ellipsis;\n  white-space: nowrap;\n  color: var(--_19ez2rz1e);\n}\n._17q7em3c ._17q7em3b {\n  opacity: 0.45;\n  font-style: italic;\n}\n._17q7em3d {\n  flex-shrink: 0;\n  font-size: 10px;\n  color: var(--_19ez2rz2);\n  background: var(--_19ez2rz1d);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: 3px;\n  padding: 0 4px;\n}\n._17q7em3e {\n  flex-shrink: 0;\n  font-size: 11px;\n  font-weight: 600;\n  margin-left: auto;\n  padding: 0 3px;\n}\n._17q7em3f {\n  display: inline-block;\n  width: 10px;\n  height: 10px;\n  border: 2px solid var(--_19ez2rze);\n  border-top-color: var(--_19ez2rzm);\n  border-radius: 50%;\n  animation: _17q7em30 0.6s linear infinite;\n  flex-shrink: 0;\n}\n._17q7em3g {\n  flex-shrink: 0;\n  color: var(--_19ez2rz10);\n  font-size: 12px;\n}\n._17q7em3h {\n  display: flex;\n  align-items: center;\n  gap: 6px;\n  padding: 6px 8px;\n  border-bottom: 1px solid var(--_19ez2rze);\n}\n._17q7em3i {\n  flex: 1 1;\n  background: var(--_19ez2rz1a);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: 4px;\n  color: var(--_19ez2rz1e);\n  font-size: 12px;\n  padding: 3px 8px;\n  outline: none;\n}\n._17q7em3i:focus {\n  border-color: var(--_19ez2rzm);\n}\n._17q7em3i::placeholder {\n  color: var(--_19ez2rz2);\n}\n._17q7em3j {\n  display: flex;\n  align-items: center;\n  gap: 4px;\n  padding: 4px 8px;\n  border-bottom: 1px solid var(--_19ez2rze);\n}\n._17q7em3k {\n  padding: 3px 8px;\n  font-size: 11px;\n  background: transparent;\n  border: 1px solid var(--_19ez2rze);\n  border-radius: 3px;\n  color: var(--_19ez2rz2);\n  cursor: pointer;\n}\n._17q7em3k:hover {\n  background: var(--_19ez2rz1k);\n  color: var(--_19ez2rz1e);\n}\n._17q7em3l {\n  display: flex;\n  align-items: center;\n  gap: 4px;\n  font-size: 11px;\n  color: var(--_19ez2rz2);\n  cursor: pointer;\n}\n._17q7em3l input[type='checkbox'] {\n  cursor: pointer;\n}\n._17q7em3m {\n  flex: 1 1;\n  overflow: hidden;\n}\n._17q7em3n {\n  background: rgba(255, 200, 0, 0.25);\n  border-radius: 2px;\n}\n._17q7em3o {\n  padding: 16px;\n  color: var(--_19ez2rz2);\n  font-style: italic;\n}\n._17q7em3p {\n  padding: 4px 8px;\n  font-size: 11px;\n  color: var(--_19ez2rz2);\n  background: var(--_19ez2rz1i);\n  border-bottom: 1px solid var(--_19ez2rze);\n}\n._17q7em3q {\n  position: absolute;\n  inset: 0;\n  display: flex;\n  align-items: center;\n  justify-content: center;\n  background: var(--_19ez2rza);\n  z-index: 10;\n}\n._17q7em3r {\n  flex-shrink: 0;\n  display: flex;\n  gap: 4px;\n  font-size: 10px;\n  margin-left: 4px;\n}\n._17q7em3s {\n  color: var(--_19ez2rz1z);\n}\n._17q7em3t {\n  color: var(--_19ez2rz20);\n}\n@media (max-width: 767px) {\n  ._17q7em36 {\n    min-height: 44px;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 58293:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".co3n740 {\n  display: flex;\n  align-items: center;\n  justify-content: space-between;\n  gap: 8px;\n  padding: 6px 10px;\n  font-size: 13px;\n  background: var(--_19ez2rzy);\n  color: var(--_19ez2rzz);\n  border-bottom: 1px solid var(--_19ez2rzx);\n}\n.co3n741 {\n  border: none;\n  background: transparent;\n  color: inherit;\n  font: inherit;\n  text-decoration: underline;\n  cursor: pointer;\n  padding: 0 2px;\n}\n@media (pointer: coarse) {\n  .co3n741 {\n    min-height: 44px;\n    min-width: 44px;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 61095:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "@keyframes _1luj7kc0 {\n  to {\n    transform: rotate(360deg);\n  }\n}\n._1luj7kc1 {\n  display: flex;\n  flex-direction: column;\n  align-items: center;\n  justify-content: center;\n  gap: 10px;\n  padding: 24px 16px;\n  color: var(--_19ez2rz1);\n  font-size: 14px;\n  text-align: center;\n}\n._1luj7kc2 {\n  width: 20px;\n  height: 20px;\n  border-radius: 50%;\n  border: 2px solid var(--_19ez2rzg);\n  border-top-color: var(--_19ez2rzm);\n  animation: _1luj7kc0 0.8s linear infinite;\n}\n._1luj7kc3 {\n  color: var(--_19ez2rz12);\n}\n._1luj7kc4 {\n  display: flex;\n  gap: 8px;\n  flex-wrap: wrap;\n  justify-content: center;\n}\n._1luj7kc5 {\n  border: 1px solid var(--_19ez2rzg);\n  border-radius: var(--_19ez2rz36);\n  background: transparent;\n  color: var(--_19ez2rz0);\n  font: inherit;\n  padding: 4px 12px;\n  cursor: pointer;\n}\n@media (prefers-reduced-motion: reduce) {\n  ._1luj7kc2 {\n    animation: none;\n  }\n}\n@media (pointer: coarse) {\n  ._1luj7kc5 {\n    min-height: 44px;\n    min-width: 44px;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 68791:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".wosrdi0 {\n  display: flex;\n  flex-direction: column;\n  flex-shrink: 0;\n}\n.wosrdi1 {\n  display: inline-flex;\n  align-items: center;\n  gap: 6px;\n  padding: 2px 8px;\n  font-size: 12px;\n  border-radius: var(--_19ez2rz36);\n  border: 1px solid var(--_19ez2rzg);\n  color: var(--_19ez2rz1);\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 73646:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  P: () => (/* binding */ Tree)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/utils.js
var utils_namespaceObject = {};
__webpack_require__.r(utils_namespaceObject);
__webpack_require__.d(utils_namespaceObject, {
  T_: () => (access),
  $s: () => (bound),
  as: () => (dfs),
  Mi: () => (focusNextElement),
  IO: () => (focusPrevElement),
  ES: () => (getInsertIndex),
  Bf: () => (getInsertParentId),
  ls: () => (identify),
  yQ: () => (identifyNull),
  qh: () => (indexOf),
  Lx: () => (isClosed),
  K9: () => (isDescendant),
  WZ: () => (isItem),
  J4: () => (isOpenWithEmptyChildren),
  FS: () => (safeRun),
  fm: () => (waitFor),
  GG: () => (walk)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/NativeTypes.js
var NativeTypes_namespaceObject = {};
__webpack_require__.r(NativeTypes_namespaceObject);
__webpack_require__.d(NativeTypes_namespaceObject, {
  FILE: () => (FILE),
  HTML: () => (HTML),
  TEXT: () => (TEXT),
  URL: () => (URL)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
var react_default = /*#__PURE__*/__webpack_require__.n(react);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/use-sync-external-store@1.6.0+55f3e2d4ca346cd1/node_modules/use-sync-external-store/shim/index.js
var shim = __webpack_require__(7446);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/context.js

const TreeApiContext = (0,react.createContext)(null);
function useTreeApi() {
    const value = (0,react.useContext)(TreeApiContext);
    if (value === null)
        throw new Error("No Tree Api Provided");
    return value;
}
const NodesContext = (0,react.createContext)(null);
function useNodesContext() {
    const value = (0,react.useContext)(NodesContext);
    if (value === null)
        throw new Error("Provide a NodesContext");
    return value;
}
const DndContext = (0,react.createContext)(null);
function useDndContext() {
    const value = (0,react.useContext)(DndContext);
    if (value === null)
        throw new Error("Provide a DnDContext");
    return value;
}
const DataUpdatesContext = (0,react.createContext)(0);
function useDataUpdates() {
    (0,react.useContext)(DataUpdatesContext);
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/utils.js
function bound(n, min, max) {
    return Math.max(Math.min(n, max), min);
}
function isItem(node) {
    return node && node.isLeaf;
}
function isClosed(node) {
    return node && node.isInternal && !node.isOpen;
}
function isOpenWithEmptyChildren(node) {
    var _a;
    return node && node.isOpen && !((_a = node.children) === null || _a === void 0 ? void 0 : _a.length);
}
/**
 * Is first param a descendant of the second param
 */
const isDescendant = (a, b) => {
    let n = a;
    while (n) {
        if (n.id === b.id)
            return true;
        n = n.parent;
    }
    return false;
};
const indexOf = (node) => {
    if (!node.parent)
        throw Error("Node does not have a parent");
    return node.parent.children.findIndex((c) => c.id === node.id);
};
function noop() { }
function dfs(node, id) {
    if (!node)
        return null;
    if (node.id === id)
        return node;
    if (node.children) {
        for (let child of node.children) {
            const result = dfs(child, id);
            if (result)
                return result;
        }
    }
    return null;
}
function walk(node, fn) {
    fn(node);
    if (node.children) {
        for (let child of node.children) {
            walk(child, fn);
        }
    }
}
function focusNextElement(target) {
    const elements = getFocusable(target);
    let next;
    for (let i = 0; i < elements.length; ++i) {
        const item = elements[i];
        if (item === target) {
            next = nextItem(elements, i);
            break;
        }
    }
    // @ts-ignore ??
    next === null || next === void 0 ? void 0 : next.focus();
}
function focusPrevElement(target) {
    const elements = getFocusable(target);
    let next;
    for (let i = 0; i < elements.length; ++i) {
        const item = elements[i];
        if (item === target) {
            next = prevItem(elements, i);
            break;
        }
    }
    // @ts-ignore
    next === null || next === void 0 ? void 0 : next.focus();
}
function nextItem(list, index) {
    if (index + 1 < list.length) {
        return list[index + 1];
    }
    else {
        return list[0];
    }
}
function prevItem(list, index) {
    if (index - 1 >= 0) {
        return list[index - 1];
    }
    else {
        return list[list.length - 1];
    }
}
function getFocusable(target) {
    return Array.from(document.querySelectorAll('button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"]):not([disabled]), details:not([disabled]), summary:not(:disabled)')).filter((e) => e === target || !target.contains(e));
}
function access(obj, accessor) {
    if (typeof accessor === "boolean")
        return accessor;
    if (typeof accessor === "string")
        return obj[accessor];
    return accessor(obj);
}
function identifyNull(obj) {
    if (obj === null)
        return null;
    else
        return identify(obj);
}
function identify(obj) {
    return typeof obj === "string" ? obj : obj.id;
}
function mergeRefs(...refs) {
    return (instance) => {
        refs.forEach((ref) => {
            if (typeof ref === "function") {
                ref(instance);
            }
            else if (ref != null) {
                ref.current = instance;
            }
        });
    };
}
function safeRun(fn, ...args) {
    if (fn)
        return fn(...args);
}
function waitFor(fn) {
    return new Promise((resolve, reject) => {
        let tries = 0;
        function check() {
            tries += 1;
            if (tries === 100)
                reject();
            if (fn())
                resolve();
            else
                setTimeout(check, 10);
        }
        check();
    });
}
function getInsertIndex(tree) {
    var _a, _b;
    const focus = tree.focusedNode;
    if (!focus)
        return (_b = (_a = tree.root.children) === null || _a === void 0 ? void 0 : _a.length) !== null && _b !== void 0 ? _b : 0;
    if (focus.isOpen)
        return 0;
    if (focus.parent)
        return focus.childIndex + 1;
    return 0;
}
function getInsertParentId(tree) {
    const focus = tree.focusedNode;
    if (!focus)
        return null;
    if (focus.isOpen)
        return focus.id;
    if (focus.parent && !focus.parent.isRoot)
        return focus.parent.id;
    return null;
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/default-cursor.js


const placeholderStyle = {
    display: "flex",
    alignItems: "center",
    zIndex: 1,
};
const lineStyle = {
    flex: 1,
    height: "2px",
    background: "#4B91E2",
    borderRadius: "1px",
};
const circleStyle = {
    width: "4px",
    height: "4px",
    boxShadow: "0 0 0 3px #4B91E2",
    borderRadius: "50%",
};
const DefaultCursor = react_default().memo(function DefaultCursor({ top, left, indent, }) {
    const style = {
        position: "absolute",
        pointerEvents: "none",
        top: top - 2 + "px",
        left: left + "px",
        right: indent + "px",
    };
    return ((0,react_jsx_runtime.jsxs)("div", { style: Object.assign(Object.assign({}, placeholderStyle), style), children: [(0,react_jsx_runtime.jsx)("div", { style: Object.assign({}, circleStyle) }), (0,react_jsx_runtime.jsx)("div", { style: Object.assign({}, lineStyle) })] }));
});

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/default-row.js

function DefaultRow({ node, attrs, innerRef, children, }) {
    return ((0,react_jsx_runtime.jsx)("div", Object.assign({}, attrs, { ref: innerRef, onFocus: (e) => e.stopPropagation(), onClick: node.handleClick, children: children })));
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/default-node.js


function DefaultNode(props) {
    return ((0,react_jsx_runtime.jsxs)("div", { ref: props.dragHandle, style: props.style, children: [(0,react_jsx_runtime.jsx)("span", { onClick: (e) => {
                    e.stopPropagation();
                    props.node.toggle();
                }, children: props.node.isLeaf ? "🌳" : props.node.isOpen ? "🗁" : "🗀" }), " ", props.node.isEditing ? (0,react_jsx_runtime.jsx)(Edit, Object.assign({}, props)) : (0,react_jsx_runtime.jsx)(Show, Object.assign({}, props))] }));
}
function Show(props) {
    return ((0,react_jsx_runtime.jsx)(react_jsx_runtime.Fragment, { children: (0,react_jsx_runtime.jsx)("span", { children: props.node.data.name }) }));
}
function Edit({ node }) {
    const input = (0,react.useRef)();
    (0,react.useEffect)(() => {
        var _a, _b;
        (_a = input.current) === null || _a === void 0 ? void 0 : _a.focus();
        (_b = input.current) === null || _b === void 0 ? void 0 : _b.select();
    }, []);
    return ((0,react_jsx_runtime.jsx)("input", { ref: input, 
        // @ts-ignore
        defaultValue: node.data.name, onBlur: () => node.reset(), onKeyDown: (e) => {
            var _a;
            if (e.key === "Escape")
                node.reset();
            if (e.key === "Enter")
                node.submit(((_a = input.current) === null || _a === void 0 ? void 0 : _a.value) || "");
        } }));
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/state/edit-slice.js
/* Actions */
function edit(id) {
    return { type: "EDIT", id };
}
/* Reducer */
function reducer(state = { id: null }, action) {
    if (action.type === "EDIT") {
        return Object.assign(Object.assign({}, state), { id: action.id });
    }
    else {
        return state;
    }
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/state/focus-slice.js
/* Types */
/* Actions */
function focus_slice_focus(id) {
    return { type: "FOCUS", id };
}
function treeBlur() {
    return { type: "TREE_BLUR" };
}
/* Reducer */
function focus_slice_reducer(state = { id: null, treeFocused: false }, action) {
    if (action.type === "FOCUS") {
        return Object.assign(Object.assign({}, state), { id: action.id, treeFocused: true });
    }
    else if (action.type === "TREE_BLUR") {
        return Object.assign(Object.assign({}, state), { treeFocused: false });
    }
    else {
        return state;
    }
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/interfaces/node-api.js

class NodeApi {
    constructor(params) {
        this.handleClick = (e) => {
            if (e.metaKey && !this.tree.props.disableMultiSelection) {
                this.isSelected ? this.deselect() : this.selectMulti();
            }
            else if (e.shiftKey && !this.tree.props.disableMultiSelection) {
                this.selectContiguous();
            }
            else {
                this.select();
                this.activate();
            }
        };
        this.tree = params.tree;
        this.id = params.id;
        this.data = params.data;
        this.level = params.level;
        this.children = params.children;
        this.parent = params.parent;
        this.isDraggable = params.isDraggable;
        this.rowIndex = params.rowIndex;
    }
    get isRoot() {
        return this.id === ROOT_ID;
    }
    get isLeaf() {
        return !Array.isArray(this.children);
    }
    get isInternal() {
        return !this.isLeaf;
    }
    get isOpen() {
        return this.isLeaf ? false : this.tree.isOpen(this.id);
    }
    get isClosed() {
        return this.isLeaf ? false : !this.tree.isOpen(this.id);
    }
    get isEditable() {
        return this.tree.isEditable(this.data);
    }
    get isEditing() {
        return this.tree.editingId === this.id;
    }
    get isSelected() {
        return this.tree.isSelected(this.id);
    }
    get isOnlySelection() {
        return this.isSelected && this.tree.hasOneSelection;
    }
    get isSelectedStart() {
        var _a;
        return this.isSelected && !((_a = this.prev) === null || _a === void 0 ? void 0 : _a.isSelected);
    }
    get isSelectedEnd() {
        var _a;
        return this.isSelected && !((_a = this.next) === null || _a === void 0 ? void 0 : _a.isSelected);
    }
    get isFocused() {
        return this.tree.isFocused(this.id);
    }
    get isDragging() {
        return this.tree.isDragging(this.id);
    }
    get willReceiveDrop() {
        return this.tree.willReceiveDrop(this.id);
    }
    get state() {
        return {
            isClosed: this.isClosed,
            isDragging: this.isDragging,
            isEditing: this.isEditing,
            isFocused: this.isFocused,
            isInternal: this.isInternal,
            isLeaf: this.isLeaf,
            isOpen: this.isOpen,
            isSelected: this.isSelected,
            isSelectedEnd: this.isSelectedEnd,
            isSelectedStart: this.isSelectedStart,
            willReceiveDrop: this.willReceiveDrop,
        };
    }
    get childIndex() {
        if (this.parent && this.parent.children) {
            return this.parent.children.findIndex((child) => child.id === this.id);
        }
        else {
            return -1;
        }
    }
    get next() {
        if (this.rowIndex === null)
            return null;
        return this.tree.at(this.rowIndex + 1);
    }
    get prev() {
        if (this.rowIndex === null)
            return null;
        return this.tree.at(this.rowIndex - 1);
    }
    get nextSibling() {
        var _a, _b;
        const i = this.childIndex;
        return (_b = (_a = this.parent) === null || _a === void 0 ? void 0 : _a.children[i + 1]) !== null && _b !== void 0 ? _b : null;
    }
    isAncestorOf(node) {
        if (!node)
            return false;
        let ancestor = node;
        while (ancestor) {
            if (ancestor.id === this.id)
                return true;
            ancestor = ancestor.parent;
        }
        return false;
    }
    select() {
        this.tree.select(this);
    }
    deselect() {
        this.tree.deselect(this);
    }
    selectMulti() {
        this.tree.selectMulti(this);
    }
    selectContiguous() {
        this.tree.selectContiguous(this);
    }
    activate() {
        this.tree.activate(this);
    }
    focus() {
        this.tree.focus(this);
    }
    toggle() {
        this.tree.toggle(this);
    }
    open() {
        this.tree.open(this);
    }
    openParents() {
        this.tree.openParents(this);
    }
    close() {
        this.tree.close(this);
    }
    submit(value) {
        this.tree.submit(this, value);
    }
    reset() {
        this.tree.reset();
    }
    clone() {
        return new NodeApi(Object.assign({}, this));
    }
    edit() {
        return this.tree.edit(this);
    }
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/data/create-root.js

const ROOT_ID = "__REACT_ARBORIST_INTERNAL_ROOT__";
function createRoot(tree) {
    var _a;
    function visitSelfAndChildren(data, level, parent) {
        const id = tree.accessId(data);
        const node = new NodeApi({
            tree,
            data,
            level,
            parent,
            id,
            children: null,
            isDraggable: tree.isDraggable(data),
            rowIndex: null,
        });
        const children = tree.accessChildren(data);
        if (children) {
            node.children = children.map((child) => visitSelfAndChildren(child, level + 1, node));
        }
        return node;
    }
    const root = new NodeApi({
        tree,
        id: ROOT_ID,
        // @ts-ignore
        data: { id: ROOT_ID },
        level: -1,
        parent: null,
        children: null,
        isDraggable: true,
        rowIndex: null,
    });
    const data = (_a = tree.props.data) !== null && _a !== void 0 ? _a : [];
    root.children = data.map((child) => {
        return visitSelfAndChildren(child, 0, root);
    });
    return root;
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/state/open-slice.js
/* Actions */
const actions = {
    open(id, filtered) {
        return { type: "VISIBILITY_OPEN", id, filtered };
    },
    close(id, filtered) {
        return { type: "VISIBILITY_CLOSE", id, filtered };
    },
    toggle(id, filtered) {
        return { type: "VISIBILITY_TOGGLE", id, filtered };
    },
    clear(filtered) {
        return { type: "VISIBILITY_CLEAR", filtered };
    },
};
/* Reducer */
function openMapReducer(state = {}, action) {
    if (action.type === "VISIBILITY_OPEN") {
        return Object.assign(Object.assign({}, state), { [action.id]: true });
    }
    else if (action.type === "VISIBILITY_CLOSE") {
        return Object.assign(Object.assign({}, state), { [action.id]: false });
    }
    else if (action.type === "VISIBILITY_TOGGLE") {
        const prev = state[action.id];
        return Object.assign(Object.assign({}, state), { [action.id]: !prev });
    }
    else if (action.type === "VISIBILITY_CLEAR") {
        return {};
    }
    else {
        return state;
    }
}
function open_slice_reducer(state = { filtered: {}, unfiltered: {} }, action) {
    if (!action.type.startsWith("VISIBILITY"))
        return state;
    if (action.filtered) {
        return Object.assign(Object.assign({}, state), { filtered: openMapReducer(state.filtered, action) });
    }
    else {
        return Object.assign(Object.assign({}, state), { unfiltered: openMapReducer(state.unfiltered, action) });
    }
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/state/initial.js
const initialState = (props) => {
    var _a;
    return ({
        nodes: {
            // Changes together
            open: { filtered: {}, unfiltered: (_a = props === null || props === void 0 ? void 0 : props.initialOpenState) !== null && _a !== void 0 ? _a : {} },
            focus: { id: null, treeFocused: false },
            edit: { id: null },
            drag: {
                id: null,
                selectedIds: [],
                destinationParentId: null,
                destinationIndex: null,
            },
            selection: { ids: new Set(), anchor: null, mostRecent: null },
        },
        dnd: {
            cursor: { type: "none" },
            dragId: null,
            dragIds: [],
            parentId: null,
            index: -1,
        },
    });
};

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/state/selection-slice.js


/* Actions */
const selection_slice_actions = {
    clear: () => ({ type: "SELECTION_CLEAR" }),
    only: (id) => ({
        type: "SELECTION_ONLY",
        id: identify(id),
    }),
    add: (id) => ({
        type: "SELECTION_ADD",
        ids: (Array.isArray(id) ? id : [id]).map(identify),
    }),
    remove: (id) => ({
        type: "SELECTION_REMOVE",
        ids: (Array.isArray(id) ? id : [id]).map(identify),
    }),
    set: (args) => (Object.assign({ type: "SELECTION_SET" }, args)),
    mostRecent: (id) => ({
        type: "SELECTION_MOST_RECENT",
        id: id === null ? null : identify(id),
    }),
    anchor: (id) => ({
        type: "SELECTION_ANCHOR",
        id: id === null ? null : identify(id),
    }),
};
/* Reducer */
function selection_slice_reducer(state = initialState()["nodes"]["selection"], action) {
    const ids = state.ids;
    switch (action.type) {
        case "SELECTION_CLEAR":
            return Object.assign(Object.assign({}, state), { ids: new Set() });
        case "SELECTION_ONLY":
            return Object.assign(Object.assign({}, state), { ids: new Set([action.id]) });
        case "SELECTION_ADD":
            if (action.ids.length === 0)
                return state;
            action.ids.forEach((id) => ids.add(id));
            return Object.assign(Object.assign({}, state), { ids: new Set(ids) });
        case "SELECTION_REMOVE":
            if (action.ids.length === 0)
                return state;
            action.ids.forEach((id) => ids.delete(id));
            return Object.assign(Object.assign({}, state), { ids: new Set(ids) });
        case "SELECTION_SET":
            return Object.assign(Object.assign({}, state), { ids: action.ids, mostRecent: action.mostRecent, anchor: action.anchor });
        case "SELECTION_MOST_RECENT":
            return Object.assign(Object.assign({}, state), { mostRecent: action.id });
        case "SELECTION_ANCHOR":
            return Object.assign(Object.assign({}, state), { anchor: action.id });
        default:
            return state;
    }
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/state/dnd-slice.js

/* Actions */
const dnd_slice_actions = {
    cursor(cursor) {
        return { type: "DND_CURSOR", cursor };
    },
    dragStart(id, dragIds) {
        return { type: "DND_DRAG_START", id, dragIds };
    },
    dragEnd() {
        return { type: "DND_DRAG_END" };
    },
    hovering(parentId, index) {
        return { type: "DND_HOVERING", parentId, index };
    },
};
/* Reducer */
function dnd_slice_reducer(state = initialState()["dnd"], action) {
    switch (action.type) {
        case "DND_CURSOR":
            return Object.assign(Object.assign({}, state), { cursor: action.cursor });
        case "DND_DRAG_START":
            return Object.assign(Object.assign({}, state), { dragId: action.id, dragIds: action.dragIds });
        case "DND_DRAG_END":
            return initialState()["dnd"];
        case "DND_HOVERING":
            return Object.assign(Object.assign({}, state), { parentId: action.parentId, index: action.index });
        default:
            return state;
    }
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/default-drag-preview.js



const layerStyles = {
    position: "fixed",
    pointerEvents: "none",
    zIndex: 100,
    left: 0,
    top: 0,
    width: "100%",
    height: "100%",
};
const getStyle = (offset) => {
    if (!offset)
        return { display: "none" };
    const { x, y } = offset;
    return { transform: `translate(${x}px, ${y}px)` };
};
const getCountStyle = (offset) => {
    if (!offset)
        return { display: "none" };
    const { x, y } = offset;
    return { transform: `translate(${x + 10}px, ${y + 10}px)` };
};
function DefaultDragPreview({ offset, mouse, id, dragIds, isDragging, }) {
    return ((0,react_jsx_runtime.jsxs)(Overlay, { isDragging: isDragging, children: [(0,react_jsx_runtime.jsx)(Position, { offset: offset, children: (0,react_jsx_runtime.jsx)(PreviewNode, { id: id, dragIds: dragIds }) }), (0,react_jsx_runtime.jsx)(Count, { mouse: mouse, count: dragIds.length })] }));
}
const Overlay = (0,react.memo)(function Overlay(props) {
    if (!props.isDragging)
        return null;
    return (0,react_jsx_runtime.jsx)("div", { style: layerStyles, children: props.children });
});
function Position(props) {
    return ((0,react_jsx_runtime.jsx)("div", { className: "row preview", style: getStyle(props.offset), children: props.children }));
}
function Count(props) {
    const { count, mouse } = props;
    if (count > 1)
        return ((0,react_jsx_runtime.jsx)("div", { className: "selected-count", style: getCountStyle(mouse), children: count }));
    else
        return null;
}
const PreviewNode = (0,react.memo)(function PreviewNode(props) {
    const tree = useTreeApi();
    const node = tree.get(props.id);
    if (!node)
        return null;
    return ((0,react_jsx_runtime.jsx)(tree.renderNode, { preview: true, node: node, style: {
            paddingLeft: node.level * tree.indent,
            opacity: 0.2,
            background: "transparent",
        }, tree: tree }));
});

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@babel+runtime@7.28.4/node_modules/@babel/runtime/helpers/esm/extends.js
function _extends() {
  return _extends = Object.assign ? Object.assign.bind() : function (n) {
    for (var e = 1; e < arguments.length; e++) {
      var t = arguments[e];
      for (var r in t) ({}).hasOwnProperty.call(t, r) && (n[r] = t[r]);
    }
    return n;
  }, _extends.apply(null, arguments);
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@babel+runtime@7.28.4/node_modules/@babel/runtime/helpers/esm/assertThisInitialized.js
function _assertThisInitialized(e) {
  if (void 0 === e) throw new ReferenceError("this hasn't been initialised - super() hasn't been called");
  return e;
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@babel+runtime@7.28.4/node_modules/@babel/runtime/helpers/esm/setPrototypeOf.js
function _setPrototypeOf(t, e) {
  return _setPrototypeOf = Object.setPrototypeOf ? Object.setPrototypeOf.bind() : function (t, e) {
    return t.__proto__ = e, t;
  }, _setPrototypeOf(t, e);
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@babel+runtime@7.28.4/node_modules/@babel/runtime/helpers/esm/inheritsLoose.js

function _inheritsLoose(t, o) {
  t.prototype = Object.create(o.prototype), t.prototype.constructor = t, _setPrototypeOf(t, o);
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/memoize-one@5.2.1/node_modules/memoize-one/dist/memoize-one.esm.js
var safeIsNaN = Number.isNaN ||
    function ponyfill(value) {
        return typeof value === 'number' && value !== value;
    };
function isEqual(first, second) {
    if (first === second) {
        return true;
    }
    if (safeIsNaN(first) && safeIsNaN(second)) {
        return true;
    }
    return false;
}
function areInputsEqual(newInputs, lastInputs) {
    if (newInputs.length !== lastInputs.length) {
        return false;
    }
    for (var i = 0; i < newInputs.length; i++) {
        if (!isEqual(newInputs[i], lastInputs[i])) {
            return false;
        }
    }
    return true;
}

function memoizeOne(resultFn, isEqual) {
    if (isEqual === void 0) { isEqual = areInputsEqual; }
    var lastThis;
    var lastArgs = [];
    var lastResult;
    var calledOnce = false;
    function memoized() {
        var newArgs = [];
        for (var _i = 0; _i < arguments.length; _i++) {
            newArgs[_i] = arguments[_i];
        }
        if (calledOnce && lastThis === this && isEqual(newArgs, lastArgs)) {
            return lastResult;
        }
        lastResult = resultFn.apply(this, newArgs);
        calledOnce = true;
        lastThis = this;
        lastArgs = newArgs;
        return lastResult;
    }
    return memoized;
}

/* harmony default export */ const memoize_one_esm = (memoizeOne);

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-window@1.8.11+498059a1009c1789/node_modules/react-window/dist/index.esm.js







// Animation frame based implementation of setTimeout.
// Inspired by Joe Lambert, https://gist.github.com/joelambert/1002116#file-requesttimeout-js
var hasNativePerformanceNow = typeof performance === 'object' && typeof performance.now === 'function';
var now = hasNativePerformanceNow ? function () {
  return performance.now();
} : function () {
  return Date.now();
};
function cancelTimeout(timeoutID) {
  cancelAnimationFrame(timeoutID.id);
}
function requestTimeout(callback, delay) {
  var start = now();

  function tick() {
    if (now() - start >= delay) {
      callback.call(null);
    } else {
      timeoutID.id = requestAnimationFrame(tick);
    }
  }

  var timeoutID = {
    id: requestAnimationFrame(tick)
  };
  return timeoutID;
}

var size = -1; // This utility copied from "dom-helpers" package.

function getScrollbarSize(recalculate) {
  if (recalculate === void 0) {
    recalculate = false;
  }

  if (size === -1 || recalculate) {
    var div = document.createElement('div');
    var style = div.style;
    style.width = '50px';
    style.height = '50px';
    style.overflow = 'scroll';
    document.body.appendChild(div);
    size = div.offsetWidth - div.clientWidth;
    document.body.removeChild(div);
  }

  return size;
}
var cachedRTLResult = null; // TRICKY According to the spec, scrollLeft should be negative for RTL aligned elements.
// Chrome does not seem to adhere; its scrollLeft values are positive (measured relative to the left).
// Safari's elastic bounce makes detecting this even more complicated wrt potential false positives.
// The safest way to check this is to intentionally set a negative offset,
// and then verify that the subsequent "scroll" event matches the negative offset.
// If it does not match, then we can assume a non-standard RTL scroll implementation.

function getRTLOffsetType(recalculate) {
  if (recalculate === void 0) {
    recalculate = false;
  }

  if (cachedRTLResult === null || recalculate) {
    var outerDiv = document.createElement('div');
    var outerStyle = outerDiv.style;
    outerStyle.width = '50px';
    outerStyle.height = '50px';
    outerStyle.overflow = 'scroll';
    outerStyle.direction = 'rtl';
    var innerDiv = document.createElement('div');
    var innerStyle = innerDiv.style;
    innerStyle.width = '100px';
    innerStyle.height = '100px';
    outerDiv.appendChild(innerDiv);
    document.body.appendChild(outerDiv);

    if (outerDiv.scrollLeft > 0) {
      cachedRTLResult = 'positive-descending';
    } else {
      outerDiv.scrollLeft = 1;

      if (outerDiv.scrollLeft === 0) {
        cachedRTLResult = 'negative';
      } else {
        cachedRTLResult = 'positive-ascending';
      }
    }

    document.body.removeChild(outerDiv);
    return cachedRTLResult;
  }

  return cachedRTLResult;
}

var IS_SCROLLING_DEBOUNCE_INTERVAL = 150;

var defaultItemKey = function defaultItemKey(_ref) {
  var columnIndex = _ref.columnIndex,
      data = _ref.data,
      rowIndex = _ref.rowIndex;
  return rowIndex + ":" + columnIndex;
}; // In DEV mode, this Set helps us only log a warning once per component instance.
// This avoids spamming the console every time a render happens.


var devWarningsOverscanCount = null;
var devWarningsOverscanRowsColumnsCount = null;
var devWarningsTagName = null;

if (false) {}

function createGridComponent(_ref2) {
  var _class;

  var getColumnOffset = _ref2.getColumnOffset,
      getColumnStartIndexForOffset = _ref2.getColumnStartIndexForOffset,
      getColumnStopIndexForStartIndex = _ref2.getColumnStopIndexForStartIndex,
      getColumnWidth = _ref2.getColumnWidth,
      getEstimatedTotalHeight = _ref2.getEstimatedTotalHeight,
      getEstimatedTotalWidth = _ref2.getEstimatedTotalWidth,
      getOffsetForColumnAndAlignment = _ref2.getOffsetForColumnAndAlignment,
      getOffsetForRowAndAlignment = _ref2.getOffsetForRowAndAlignment,
      getRowHeight = _ref2.getRowHeight,
      getRowOffset = _ref2.getRowOffset,
      getRowStartIndexForOffset = _ref2.getRowStartIndexForOffset,
      getRowStopIndexForStartIndex = _ref2.getRowStopIndexForStartIndex,
      initInstanceProps = _ref2.initInstanceProps,
      shouldResetStyleCacheOnItemSizeChange = _ref2.shouldResetStyleCacheOnItemSizeChange,
      validateProps = _ref2.validateProps;
  return _class = /*#__PURE__*/function (_PureComponent) {
    _inheritsLoose(Grid, _PureComponent);

    // Always use explicit constructor for React components.
    // It produces less code after transpilation. (#26)
    // eslint-disable-next-line no-useless-constructor
    function Grid(props) {
      var _this;

      _this = _PureComponent.call(this, props) || this;
      _this._instanceProps = initInstanceProps(_this.props, _assertThisInitialized(_this));
      _this._resetIsScrollingTimeoutId = null;
      _this._outerRef = void 0;
      _this.state = {
        instance: _assertThisInitialized(_this),
        isScrolling: false,
        horizontalScrollDirection: 'forward',
        scrollLeft: typeof _this.props.initialScrollLeft === 'number' ? _this.props.initialScrollLeft : 0,
        scrollTop: typeof _this.props.initialScrollTop === 'number' ? _this.props.initialScrollTop : 0,
        scrollUpdateWasRequested: false,
        verticalScrollDirection: 'forward'
      };
      _this._callOnItemsRendered = void 0;
      _this._callOnItemsRendered = memoize_one_esm(function (overscanColumnStartIndex, overscanColumnStopIndex, overscanRowStartIndex, overscanRowStopIndex, visibleColumnStartIndex, visibleColumnStopIndex, visibleRowStartIndex, visibleRowStopIndex) {
        return _this.props.onItemsRendered({
          overscanColumnStartIndex: overscanColumnStartIndex,
          overscanColumnStopIndex: overscanColumnStopIndex,
          overscanRowStartIndex: overscanRowStartIndex,
          overscanRowStopIndex: overscanRowStopIndex,
          visibleColumnStartIndex: visibleColumnStartIndex,
          visibleColumnStopIndex: visibleColumnStopIndex,
          visibleRowStartIndex: visibleRowStartIndex,
          visibleRowStopIndex: visibleRowStopIndex
        });
      });
      _this._callOnScroll = void 0;
      _this._callOnScroll = memoize_one_esm(function (scrollLeft, scrollTop, horizontalScrollDirection, verticalScrollDirection, scrollUpdateWasRequested) {
        return _this.props.onScroll({
          horizontalScrollDirection: horizontalScrollDirection,
          scrollLeft: scrollLeft,
          scrollTop: scrollTop,
          verticalScrollDirection: verticalScrollDirection,
          scrollUpdateWasRequested: scrollUpdateWasRequested
        });
      });
      _this._getItemStyle = void 0;

      _this._getItemStyle = function (rowIndex, columnIndex) {
        var _this$props = _this.props,
            columnWidth = _this$props.columnWidth,
            direction = _this$props.direction,
            rowHeight = _this$props.rowHeight;

        var itemStyleCache = _this._getItemStyleCache(shouldResetStyleCacheOnItemSizeChange && columnWidth, shouldResetStyleCacheOnItemSizeChange && direction, shouldResetStyleCacheOnItemSizeChange && rowHeight);

        var key = rowIndex + ":" + columnIndex;
        var style;

        if (itemStyleCache.hasOwnProperty(key)) {
          style = itemStyleCache[key];
        } else {
          var _offset = getColumnOffset(_this.props, columnIndex, _this._instanceProps);

          var isRtl = direction === 'rtl';
          itemStyleCache[key] = style = {
            position: 'absolute',
            left: isRtl ? undefined : _offset,
            right: isRtl ? _offset : undefined,
            top: getRowOffset(_this.props, rowIndex, _this._instanceProps),
            height: getRowHeight(_this.props, rowIndex, _this._instanceProps),
            width: getColumnWidth(_this.props, columnIndex, _this._instanceProps)
          };
        }

        return style;
      };

      _this._getItemStyleCache = void 0;
      _this._getItemStyleCache = memoize_one_esm(function (_, __, ___) {
        return {};
      });

      _this._onScroll = function (event) {
        var _event$currentTarget = event.currentTarget,
            clientHeight = _event$currentTarget.clientHeight,
            clientWidth = _event$currentTarget.clientWidth,
            scrollLeft = _event$currentTarget.scrollLeft,
            scrollTop = _event$currentTarget.scrollTop,
            scrollHeight = _event$currentTarget.scrollHeight,
            scrollWidth = _event$currentTarget.scrollWidth;

        _this.setState(function (prevState) {
          if (prevState.scrollLeft === scrollLeft && prevState.scrollTop === scrollTop) {
            // Scroll position may have been updated by cDM/cDU,
            // In which case we don't need to trigger another render,
            // And we don't want to update state.isScrolling.
            return null;
          }

          var direction = _this.props.direction; // TRICKY According to the spec, scrollLeft should be negative for RTL aligned elements.
          // This is not the case for all browsers though (e.g. Chrome reports values as positive, measured relative to the left).
          // It's also easier for this component if we convert offsets to the same format as they would be in for ltr.
          // So the simplest solution is to determine which browser behavior we're dealing with, and convert based on it.

          var calculatedScrollLeft = scrollLeft;

          if (direction === 'rtl') {
            switch (getRTLOffsetType()) {
              case 'negative':
                calculatedScrollLeft = -scrollLeft;
                break;

              case 'positive-descending':
                calculatedScrollLeft = scrollWidth - clientWidth - scrollLeft;
                break;
            }
          } // Prevent Safari's elastic scrolling from causing visual shaking when scrolling past bounds.


          calculatedScrollLeft = Math.max(0, Math.min(calculatedScrollLeft, scrollWidth - clientWidth));
          var calculatedScrollTop = Math.max(0, Math.min(scrollTop, scrollHeight - clientHeight));
          return {
            isScrolling: true,
            horizontalScrollDirection: prevState.scrollLeft < scrollLeft ? 'forward' : 'backward',
            scrollLeft: calculatedScrollLeft,
            scrollTop: calculatedScrollTop,
            verticalScrollDirection: prevState.scrollTop < scrollTop ? 'forward' : 'backward',
            scrollUpdateWasRequested: false
          };
        }, _this._resetIsScrollingDebounced);
      };

      _this._outerRefSetter = function (ref) {
        var outerRef = _this.props.outerRef;
        _this._outerRef = ref;

        if (typeof outerRef === 'function') {
          outerRef(ref);
        } else if (outerRef != null && typeof outerRef === 'object' && outerRef.hasOwnProperty('current')) {
          outerRef.current = ref;
        }
      };

      _this._resetIsScrollingDebounced = function () {
        if (_this._resetIsScrollingTimeoutId !== null) {
          cancelTimeout(_this._resetIsScrollingTimeoutId);
        }

        _this._resetIsScrollingTimeoutId = requestTimeout(_this._resetIsScrolling, IS_SCROLLING_DEBOUNCE_INTERVAL);
      };

      _this._resetIsScrolling = function () {
        _this._resetIsScrollingTimeoutId = null;

        _this.setState({
          isScrolling: false
        }, function () {
          // Clear style cache after state update has been committed.
          // This way we don't break pure sCU for items that don't use isScrolling param.
          _this._getItemStyleCache(-1);
        });
      };

      return _this;
    }

    Grid.getDerivedStateFromProps = function getDerivedStateFromProps(nextProps, prevState) {
      validateSharedProps(nextProps, prevState);
      validateProps(nextProps);
      return null;
    };

    var _proto = Grid.prototype;

    _proto.scrollTo = function scrollTo(_ref3) {
      var scrollLeft = _ref3.scrollLeft,
          scrollTop = _ref3.scrollTop;

      if (scrollLeft !== undefined) {
        scrollLeft = Math.max(0, scrollLeft);
      }

      if (scrollTop !== undefined) {
        scrollTop = Math.max(0, scrollTop);
      }

      this.setState(function (prevState) {
        if (scrollLeft === undefined) {
          scrollLeft = prevState.scrollLeft;
        }

        if (scrollTop === undefined) {
          scrollTop = prevState.scrollTop;
        }

        if (prevState.scrollLeft === scrollLeft && prevState.scrollTop === scrollTop) {
          return null;
        }

        return {
          horizontalScrollDirection: prevState.scrollLeft < scrollLeft ? 'forward' : 'backward',
          scrollLeft: scrollLeft,
          scrollTop: scrollTop,
          scrollUpdateWasRequested: true,
          verticalScrollDirection: prevState.scrollTop < scrollTop ? 'forward' : 'backward'
        };
      }, this._resetIsScrollingDebounced);
    };

    _proto.scrollToItem = function scrollToItem(_ref4) {
      var _ref4$align = _ref4.align,
          align = _ref4$align === void 0 ? 'auto' : _ref4$align,
          columnIndex = _ref4.columnIndex,
          rowIndex = _ref4.rowIndex;
      var _this$props2 = this.props,
          columnCount = _this$props2.columnCount,
          height = _this$props2.height,
          rowCount = _this$props2.rowCount,
          width = _this$props2.width;
      var _this$state = this.state,
          scrollLeft = _this$state.scrollLeft,
          scrollTop = _this$state.scrollTop;
      var scrollbarSize = getScrollbarSize();

      if (columnIndex !== undefined) {
        columnIndex = Math.max(0, Math.min(columnIndex, columnCount - 1));
      }

      if (rowIndex !== undefined) {
        rowIndex = Math.max(0, Math.min(rowIndex, rowCount - 1));
      }

      var estimatedTotalHeight = getEstimatedTotalHeight(this.props, this._instanceProps);
      var estimatedTotalWidth = getEstimatedTotalWidth(this.props, this._instanceProps); // The scrollbar size should be considered when scrolling an item into view,
      // to ensure it's fully visible.
      // But we only need to account for its size when it's actually visible.

      var horizontalScrollbarSize = estimatedTotalWidth > width ? scrollbarSize : 0;
      var verticalScrollbarSize = estimatedTotalHeight > height ? scrollbarSize : 0;
      this.scrollTo({
        scrollLeft: columnIndex !== undefined ? getOffsetForColumnAndAlignment(this.props, columnIndex, align, scrollLeft, this._instanceProps, verticalScrollbarSize) : scrollLeft,
        scrollTop: rowIndex !== undefined ? getOffsetForRowAndAlignment(this.props, rowIndex, align, scrollTop, this._instanceProps, horizontalScrollbarSize) : scrollTop
      });
    };

    _proto.componentDidMount = function componentDidMount() {
      var _this$props3 = this.props,
          initialScrollLeft = _this$props3.initialScrollLeft,
          initialScrollTop = _this$props3.initialScrollTop;

      if (this._outerRef != null) {
        var outerRef = this._outerRef;

        if (typeof initialScrollLeft === 'number') {
          outerRef.scrollLeft = initialScrollLeft;
        }

        if (typeof initialScrollTop === 'number') {
          outerRef.scrollTop = initialScrollTop;
        }
      }

      this._callPropsCallbacks();
    };

    _proto.componentDidUpdate = function componentDidUpdate() {
      var direction = this.props.direction;
      var _this$state2 = this.state,
          scrollLeft = _this$state2.scrollLeft,
          scrollTop = _this$state2.scrollTop,
          scrollUpdateWasRequested = _this$state2.scrollUpdateWasRequested;

      if (scrollUpdateWasRequested && this._outerRef != null) {
        // TRICKY According to the spec, scrollLeft should be negative for RTL aligned elements.
        // This is not the case for all browsers though (e.g. Chrome reports values as positive, measured relative to the left).
        // So we need to determine which browser behavior we're dealing with, and mimic it.
        var outerRef = this._outerRef;

        if (direction === 'rtl') {
          switch (getRTLOffsetType()) {
            case 'negative':
              outerRef.scrollLeft = -scrollLeft;
              break;

            case 'positive-ascending':
              outerRef.scrollLeft = scrollLeft;
              break;

            default:
              var clientWidth = outerRef.clientWidth,
                  scrollWidth = outerRef.scrollWidth;
              outerRef.scrollLeft = scrollWidth - clientWidth - scrollLeft;
              break;
          }
        } else {
          outerRef.scrollLeft = Math.max(0, scrollLeft);
        }

        outerRef.scrollTop = Math.max(0, scrollTop);
      }

      this._callPropsCallbacks();
    };

    _proto.componentWillUnmount = function componentWillUnmount() {
      if (this._resetIsScrollingTimeoutId !== null) {
        cancelTimeout(this._resetIsScrollingTimeoutId);
      }
    };

    _proto.render = function render() {
      var _this$props4 = this.props,
          children = _this$props4.children,
          className = _this$props4.className,
          columnCount = _this$props4.columnCount,
          direction = _this$props4.direction,
          height = _this$props4.height,
          innerRef = _this$props4.innerRef,
          innerElementType = _this$props4.innerElementType,
          innerTagName = _this$props4.innerTagName,
          itemData = _this$props4.itemData,
          _this$props4$itemKey = _this$props4.itemKey,
          itemKey = _this$props4$itemKey === void 0 ? defaultItemKey : _this$props4$itemKey,
          outerElementType = _this$props4.outerElementType,
          outerTagName = _this$props4.outerTagName,
          rowCount = _this$props4.rowCount,
          style = _this$props4.style,
          useIsScrolling = _this$props4.useIsScrolling,
          width = _this$props4.width;
      var isScrolling = this.state.isScrolling;

      var _this$_getHorizontalR = this._getHorizontalRangeToRender(),
          columnStartIndex = _this$_getHorizontalR[0],
          columnStopIndex = _this$_getHorizontalR[1];

      var _this$_getVerticalRan = this._getVerticalRangeToRender(),
          rowStartIndex = _this$_getVerticalRan[0],
          rowStopIndex = _this$_getVerticalRan[1];

      var items = [];

      if (columnCount > 0 && rowCount) {
        for (var _rowIndex = rowStartIndex; _rowIndex <= rowStopIndex; _rowIndex++) {
          for (var _columnIndex = columnStartIndex; _columnIndex <= columnStopIndex; _columnIndex++) {
            items.push((0,react.createElement)(children, {
              columnIndex: _columnIndex,
              data: itemData,
              isScrolling: useIsScrolling ? isScrolling : undefined,
              key: itemKey({
                columnIndex: _columnIndex,
                data: itemData,
                rowIndex: _rowIndex
              }),
              rowIndex: _rowIndex,
              style: this._getItemStyle(_rowIndex, _columnIndex)
            }));
          }
        }
      } // Read this value AFTER items have been created,
      // So their actual sizes (if variable) are taken into consideration.


      var estimatedTotalHeight = getEstimatedTotalHeight(this.props, this._instanceProps);
      var estimatedTotalWidth = getEstimatedTotalWidth(this.props, this._instanceProps);
      return (0,react.createElement)(outerElementType || outerTagName || 'div', {
        className: className,
        onScroll: this._onScroll,
        ref: this._outerRefSetter,
        style: _extends({
          position: 'relative',
          height: height,
          width: width,
          overflow: 'auto',
          WebkitOverflowScrolling: 'touch',
          willChange: 'transform',
          direction: direction
        }, style)
      }, (0,react.createElement)(innerElementType || innerTagName || 'div', {
        children: items,
        ref: innerRef,
        style: {
          height: estimatedTotalHeight,
          pointerEvents: isScrolling ? 'none' : undefined,
          width: estimatedTotalWidth
        }
      }));
    };

    _proto._callPropsCallbacks = function _callPropsCallbacks() {
      var _this$props5 = this.props,
          columnCount = _this$props5.columnCount,
          onItemsRendered = _this$props5.onItemsRendered,
          onScroll = _this$props5.onScroll,
          rowCount = _this$props5.rowCount;

      if (typeof onItemsRendered === 'function') {
        if (columnCount > 0 && rowCount > 0) {
          var _this$_getHorizontalR2 = this._getHorizontalRangeToRender(),
              _overscanColumnStartIndex = _this$_getHorizontalR2[0],
              _overscanColumnStopIndex = _this$_getHorizontalR2[1],
              _visibleColumnStartIndex = _this$_getHorizontalR2[2],
              _visibleColumnStopIndex = _this$_getHorizontalR2[3];

          var _this$_getVerticalRan2 = this._getVerticalRangeToRender(),
              _overscanRowStartIndex = _this$_getVerticalRan2[0],
              _overscanRowStopIndex = _this$_getVerticalRan2[1],
              _visibleRowStartIndex = _this$_getVerticalRan2[2],
              _visibleRowStopIndex = _this$_getVerticalRan2[3];

          this._callOnItemsRendered(_overscanColumnStartIndex, _overscanColumnStopIndex, _overscanRowStartIndex, _overscanRowStopIndex, _visibleColumnStartIndex, _visibleColumnStopIndex, _visibleRowStartIndex, _visibleRowStopIndex);
        }
      }

      if (typeof onScroll === 'function') {
        var _this$state3 = this.state,
            _horizontalScrollDirection = _this$state3.horizontalScrollDirection,
            _scrollLeft = _this$state3.scrollLeft,
            _scrollTop = _this$state3.scrollTop,
            _scrollUpdateWasRequested = _this$state3.scrollUpdateWasRequested,
            _verticalScrollDirection = _this$state3.verticalScrollDirection;

        this._callOnScroll(_scrollLeft, _scrollTop, _horizontalScrollDirection, _verticalScrollDirection, _scrollUpdateWasRequested);
      }
    } // Lazily create and cache item styles while scrolling,
    // So that pure component sCU will prevent re-renders.
    // We maintain this cache, and pass a style prop rather than index,
    // So that List can clear cached styles and force item re-render if necessary.
    ;

    _proto._getHorizontalRangeToRender = function _getHorizontalRangeToRender() {
      var _this$props6 = this.props,
          columnCount = _this$props6.columnCount,
          overscanColumnCount = _this$props6.overscanColumnCount,
          overscanColumnsCount = _this$props6.overscanColumnsCount,
          overscanCount = _this$props6.overscanCount,
          rowCount = _this$props6.rowCount;
      var _this$state4 = this.state,
          horizontalScrollDirection = _this$state4.horizontalScrollDirection,
          isScrolling = _this$state4.isScrolling,
          scrollLeft = _this$state4.scrollLeft;
      var overscanCountResolved = overscanColumnCount || overscanColumnsCount || overscanCount || 1;

      if (columnCount === 0 || rowCount === 0) {
        return [0, 0, 0, 0];
      }

      var startIndex = getColumnStartIndexForOffset(this.props, scrollLeft, this._instanceProps);
      var stopIndex = getColumnStopIndexForStartIndex(this.props, startIndex, scrollLeft, this._instanceProps); // Overscan by one item in each direction so that tab/focus works.
      // If there isn't at least one extra item, tab loops back around.

      var overscanBackward = !isScrolling || horizontalScrollDirection === 'backward' ? Math.max(1, overscanCountResolved) : 1;
      var overscanForward = !isScrolling || horizontalScrollDirection === 'forward' ? Math.max(1, overscanCountResolved) : 1;
      return [Math.max(0, startIndex - overscanBackward), Math.max(0, Math.min(columnCount - 1, stopIndex + overscanForward)), startIndex, stopIndex];
    };

    _proto._getVerticalRangeToRender = function _getVerticalRangeToRender() {
      var _this$props7 = this.props,
          columnCount = _this$props7.columnCount,
          overscanCount = _this$props7.overscanCount,
          overscanRowCount = _this$props7.overscanRowCount,
          overscanRowsCount = _this$props7.overscanRowsCount,
          rowCount = _this$props7.rowCount;
      var _this$state5 = this.state,
          isScrolling = _this$state5.isScrolling,
          verticalScrollDirection = _this$state5.verticalScrollDirection,
          scrollTop = _this$state5.scrollTop;
      var overscanCountResolved = overscanRowCount || overscanRowsCount || overscanCount || 1;

      if (columnCount === 0 || rowCount === 0) {
        return [0, 0, 0, 0];
      }

      var startIndex = getRowStartIndexForOffset(this.props, scrollTop, this._instanceProps);
      var stopIndex = getRowStopIndexForStartIndex(this.props, startIndex, scrollTop, this._instanceProps); // Overscan by one item in each direction so that tab/focus works.
      // If there isn't at least one extra item, tab loops back around.

      var overscanBackward = !isScrolling || verticalScrollDirection === 'backward' ? Math.max(1, overscanCountResolved) : 1;
      var overscanForward = !isScrolling || verticalScrollDirection === 'forward' ? Math.max(1, overscanCountResolved) : 1;
      return [Math.max(0, startIndex - overscanBackward), Math.max(0, Math.min(rowCount - 1, stopIndex + overscanForward)), startIndex, stopIndex];
    };

    return Grid;
  }(react.PureComponent), _class.defaultProps = {
    direction: 'ltr',
    itemData: undefined,
    useIsScrolling: false
  }, _class;
}

var validateSharedProps = function validateSharedProps(_ref5, _ref6) {
  var children = _ref5.children,
      direction = _ref5.direction,
      height = _ref5.height,
      innerTagName = _ref5.innerTagName,
      outerTagName = _ref5.outerTagName,
      overscanColumnsCount = _ref5.overscanColumnsCount,
      overscanCount = _ref5.overscanCount,
      overscanRowsCount = _ref5.overscanRowsCount,
      width = _ref5.width;
  var instance = _ref6.instance;

  if (false) {}
};

var DEFAULT_ESTIMATED_ITEM_SIZE = 50;

var getEstimatedTotalHeight = function getEstimatedTotalHeight(_ref, _ref2) {
  var rowCount = _ref.rowCount;
  var rowMetadataMap = _ref2.rowMetadataMap,
      estimatedRowHeight = _ref2.estimatedRowHeight,
      lastMeasuredRowIndex = _ref2.lastMeasuredRowIndex;
  var totalSizeOfMeasuredRows = 0; // Edge case check for when the number of items decreases while a scroll is in progress.
  // https://github.com/bvaughn/react-window/pull/138

  if (lastMeasuredRowIndex >= rowCount) {
    lastMeasuredRowIndex = rowCount - 1;
  }

  if (lastMeasuredRowIndex >= 0) {
    var itemMetadata = rowMetadataMap[lastMeasuredRowIndex];
    totalSizeOfMeasuredRows = itemMetadata.offset + itemMetadata.size;
  }

  var numUnmeasuredItems = rowCount - lastMeasuredRowIndex - 1;
  var totalSizeOfUnmeasuredItems = numUnmeasuredItems * estimatedRowHeight;
  return totalSizeOfMeasuredRows + totalSizeOfUnmeasuredItems;
};

var getEstimatedTotalWidth = function getEstimatedTotalWidth(_ref3, _ref4) {
  var columnCount = _ref3.columnCount;
  var columnMetadataMap = _ref4.columnMetadataMap,
      estimatedColumnWidth = _ref4.estimatedColumnWidth,
      lastMeasuredColumnIndex = _ref4.lastMeasuredColumnIndex;
  var totalSizeOfMeasuredRows = 0; // Edge case check for when the number of items decreases while a scroll is in progress.
  // https://github.com/bvaughn/react-window/pull/138

  if (lastMeasuredColumnIndex >= columnCount) {
    lastMeasuredColumnIndex = columnCount - 1;
  }

  if (lastMeasuredColumnIndex >= 0) {
    var itemMetadata = columnMetadataMap[lastMeasuredColumnIndex];
    totalSizeOfMeasuredRows = itemMetadata.offset + itemMetadata.size;
  }

  var numUnmeasuredItems = columnCount - lastMeasuredColumnIndex - 1;
  var totalSizeOfUnmeasuredItems = numUnmeasuredItems * estimatedColumnWidth;
  return totalSizeOfMeasuredRows + totalSizeOfUnmeasuredItems;
};

var getItemMetadata = function getItemMetadata(itemType, props, index, instanceProps) {
  var itemMetadataMap, itemSize, lastMeasuredIndex;

  if (itemType === 'column') {
    itemMetadataMap = instanceProps.columnMetadataMap;
    itemSize = props.columnWidth;
    lastMeasuredIndex = instanceProps.lastMeasuredColumnIndex;
  } else {
    itemMetadataMap = instanceProps.rowMetadataMap;
    itemSize = props.rowHeight;
    lastMeasuredIndex = instanceProps.lastMeasuredRowIndex;
  }

  if (index > lastMeasuredIndex) {
    var offset = 0;

    if (lastMeasuredIndex >= 0) {
      var itemMetadata = itemMetadataMap[lastMeasuredIndex];
      offset = itemMetadata.offset + itemMetadata.size;
    }

    for (var i = lastMeasuredIndex + 1; i <= index; i++) {
      var size = itemSize(i);
      itemMetadataMap[i] = {
        offset: offset,
        size: size
      };
      offset += size;
    }

    if (itemType === 'column') {
      instanceProps.lastMeasuredColumnIndex = index;
    } else {
      instanceProps.lastMeasuredRowIndex = index;
    }
  }

  return itemMetadataMap[index];
};

var findNearestItem = function findNearestItem(itemType, props, instanceProps, offset) {
  var itemMetadataMap, lastMeasuredIndex;

  if (itemType === 'column') {
    itemMetadataMap = instanceProps.columnMetadataMap;
    lastMeasuredIndex = instanceProps.lastMeasuredColumnIndex;
  } else {
    itemMetadataMap = instanceProps.rowMetadataMap;
    lastMeasuredIndex = instanceProps.lastMeasuredRowIndex;
  }

  var lastMeasuredItemOffset = lastMeasuredIndex > 0 ? itemMetadataMap[lastMeasuredIndex].offset : 0;

  if (lastMeasuredItemOffset >= offset) {
    // If we've already measured items within this range just use a binary search as it's faster.
    return findNearestItemBinarySearch(itemType, props, instanceProps, lastMeasuredIndex, 0, offset);
  } else {
    // If we haven't yet measured this high, fallback to an exponential search with an inner binary search.
    // The exponential search avoids pre-computing sizes for the full set of items as a binary search would.
    // The overall complexity for this approach is O(log n).
    return findNearestItemExponentialSearch(itemType, props, instanceProps, Math.max(0, lastMeasuredIndex), offset);
  }
};

var findNearestItemBinarySearch = function findNearestItemBinarySearch(itemType, props, instanceProps, high, low, offset) {
  while (low <= high) {
    var middle = low + Math.floor((high - low) / 2);
    var currentOffset = getItemMetadata(itemType, props, middle, instanceProps).offset;

    if (currentOffset === offset) {
      return middle;
    } else if (currentOffset < offset) {
      low = middle + 1;
    } else if (currentOffset > offset) {
      high = middle - 1;
    }
  }

  if (low > 0) {
    return low - 1;
  } else {
    return 0;
  }
};

var findNearestItemExponentialSearch = function findNearestItemExponentialSearch(itemType, props, instanceProps, index, offset) {
  var itemCount = itemType === 'column' ? props.columnCount : props.rowCount;
  var interval = 1;

  while (index < itemCount && getItemMetadata(itemType, props, index, instanceProps).offset < offset) {
    index += interval;
    interval *= 2;
  }

  return findNearestItemBinarySearch(itemType, props, instanceProps, Math.min(index, itemCount - 1), Math.floor(index / 2), offset);
};

var getOffsetForIndexAndAlignment = function getOffsetForIndexAndAlignment(itemType, props, index, align, scrollOffset, instanceProps, scrollbarSize) {
  var size = itemType === 'column' ? props.width : props.height;
  var itemMetadata = getItemMetadata(itemType, props, index, instanceProps); // Get estimated total size after ItemMetadata is computed,
  // To ensure it reflects actual measurements instead of just estimates.

  var estimatedTotalSize = itemType === 'column' ? getEstimatedTotalWidth(props, instanceProps) : getEstimatedTotalHeight(props, instanceProps);
  var maxOffset = Math.max(0, Math.min(estimatedTotalSize - size, itemMetadata.offset));
  var minOffset = Math.max(0, itemMetadata.offset - size + scrollbarSize + itemMetadata.size);

  if (align === 'smart') {
    if (scrollOffset >= minOffset - size && scrollOffset <= maxOffset + size) {
      align = 'auto';
    } else {
      align = 'center';
    }
  }

  switch (align) {
    case 'start':
      return maxOffset;

    case 'end':
      return minOffset;

    case 'center':
      return Math.round(minOffset + (maxOffset - minOffset) / 2);

    case 'auto':
    default:
      if (scrollOffset >= minOffset && scrollOffset <= maxOffset) {
        return scrollOffset;
      } else if (minOffset > maxOffset) {
        // Because we only take into account the scrollbar size when calculating minOffset
        // this value can be larger than maxOffset when at the end of the list
        return minOffset;
      } else if (scrollOffset < minOffset) {
        return minOffset;
      } else {
        return maxOffset;
      }

  }
};

var VariableSizeGrid = /*#__PURE__*/createGridComponent({
  getColumnOffset: function getColumnOffset(props, index, instanceProps) {
    return getItemMetadata('column', props, index, instanceProps).offset;
  },
  getColumnStartIndexForOffset: function getColumnStartIndexForOffset(props, scrollLeft, instanceProps) {
    return findNearestItem('column', props, instanceProps, scrollLeft);
  },
  getColumnStopIndexForStartIndex: function getColumnStopIndexForStartIndex(props, startIndex, scrollLeft, instanceProps) {
    var columnCount = props.columnCount,
        width = props.width;
    var itemMetadata = getItemMetadata('column', props, startIndex, instanceProps);
    var maxOffset = scrollLeft + width;
    var offset = itemMetadata.offset + itemMetadata.size;
    var stopIndex = startIndex;

    while (stopIndex < columnCount - 1 && offset < maxOffset) {
      stopIndex++;
      offset += getItemMetadata('column', props, stopIndex, instanceProps).size;
    }

    return stopIndex;
  },
  getColumnWidth: function getColumnWidth(props, index, instanceProps) {
    return instanceProps.columnMetadataMap[index].size;
  },
  getEstimatedTotalHeight: getEstimatedTotalHeight,
  getEstimatedTotalWidth: getEstimatedTotalWidth,
  getOffsetForColumnAndAlignment: function getOffsetForColumnAndAlignment(props, index, align, scrollOffset, instanceProps, scrollbarSize) {
    return getOffsetForIndexAndAlignment('column', props, index, align, scrollOffset, instanceProps, scrollbarSize);
  },
  getOffsetForRowAndAlignment: function getOffsetForRowAndAlignment(props, index, align, scrollOffset, instanceProps, scrollbarSize) {
    return getOffsetForIndexAndAlignment('row', props, index, align, scrollOffset, instanceProps, scrollbarSize);
  },
  getRowOffset: function getRowOffset(props, index, instanceProps) {
    return getItemMetadata('row', props, index, instanceProps).offset;
  },
  getRowHeight: function getRowHeight(props, index, instanceProps) {
    return instanceProps.rowMetadataMap[index].size;
  },
  getRowStartIndexForOffset: function getRowStartIndexForOffset(props, scrollTop, instanceProps) {
    return findNearestItem('row', props, instanceProps, scrollTop);
  },
  getRowStopIndexForStartIndex: function getRowStopIndexForStartIndex(props, startIndex, scrollTop, instanceProps) {
    var rowCount = props.rowCount,
        height = props.height;
    var itemMetadata = getItemMetadata('row', props, startIndex, instanceProps);
    var maxOffset = scrollTop + height;
    var offset = itemMetadata.offset + itemMetadata.size;
    var stopIndex = startIndex;

    while (stopIndex < rowCount - 1 && offset < maxOffset) {
      stopIndex++;
      offset += getItemMetadata('row', props, stopIndex, instanceProps).size;
    }

    return stopIndex;
  },
  initInstanceProps: function initInstanceProps(props, instance) {
    var _ref5 = props,
        estimatedColumnWidth = _ref5.estimatedColumnWidth,
        estimatedRowHeight = _ref5.estimatedRowHeight;
    var instanceProps = {
      columnMetadataMap: {},
      estimatedColumnWidth: estimatedColumnWidth || DEFAULT_ESTIMATED_ITEM_SIZE,
      estimatedRowHeight: estimatedRowHeight || DEFAULT_ESTIMATED_ITEM_SIZE,
      lastMeasuredColumnIndex: -1,
      lastMeasuredRowIndex: -1,
      rowMetadataMap: {}
    };

    instance.resetAfterColumnIndex = function (columnIndex, shouldForceUpdate) {
      if (shouldForceUpdate === void 0) {
        shouldForceUpdate = true;
      }

      instance.resetAfterIndices({
        columnIndex: columnIndex,
        shouldForceUpdate: shouldForceUpdate
      });
    };

    instance.resetAfterRowIndex = function (rowIndex, shouldForceUpdate) {
      if (shouldForceUpdate === void 0) {
        shouldForceUpdate = true;
      }

      instance.resetAfterIndices({
        rowIndex: rowIndex,
        shouldForceUpdate: shouldForceUpdate
      });
    };

    instance.resetAfterIndices = function (_ref6) {
      var columnIndex = _ref6.columnIndex,
          rowIndex = _ref6.rowIndex,
          _ref6$shouldForceUpda = _ref6.shouldForceUpdate,
          shouldForceUpdate = _ref6$shouldForceUpda === void 0 ? true : _ref6$shouldForceUpda;

      if (typeof columnIndex === 'number') {
        instanceProps.lastMeasuredColumnIndex = Math.min(instanceProps.lastMeasuredColumnIndex, columnIndex - 1);
      }

      if (typeof rowIndex === 'number') {
        instanceProps.lastMeasuredRowIndex = Math.min(instanceProps.lastMeasuredRowIndex, rowIndex - 1);
      } // We could potentially optimize further by only evicting styles after this index,
      // But since styles are only cached while scrolling is in progress-
      // It seems an unnecessary optimization.
      // It's unlikely that resetAfterIndex() will be called while a user is scrolling.


      instance._getItemStyleCache(-1);

      if (shouldForceUpdate) {
        instance.forceUpdate();
      }
    };

    return instanceProps;
  },
  shouldResetStyleCacheOnItemSizeChange: false,
  validateProps: function validateProps(_ref7) {
    var columnWidth = _ref7.columnWidth,
        rowHeight = _ref7.rowHeight;

    if (false) {}
  }
});

var IS_SCROLLING_DEBOUNCE_INTERVAL$1 = 150;

var defaultItemKey$1 = function defaultItemKey(index, data) {
  return index;
}; // In DEV mode, this Set helps us only log a warning once per component instance.
// This avoids spamming the console every time a render happens.


var devWarningsDirection = null;
var devWarningsTagName$1 = null;

if (false) {}

function createListComponent(_ref) {
  var _class;

  var getItemOffset = _ref.getItemOffset,
      getEstimatedTotalSize = _ref.getEstimatedTotalSize,
      getItemSize = _ref.getItemSize,
      getOffsetForIndexAndAlignment = _ref.getOffsetForIndexAndAlignment,
      getStartIndexForOffset = _ref.getStartIndexForOffset,
      getStopIndexForStartIndex = _ref.getStopIndexForStartIndex,
      initInstanceProps = _ref.initInstanceProps,
      shouldResetStyleCacheOnItemSizeChange = _ref.shouldResetStyleCacheOnItemSizeChange,
      validateProps = _ref.validateProps;
  return _class = /*#__PURE__*/function (_PureComponent) {
    _inheritsLoose(List, _PureComponent);

    // Always use explicit constructor for React components.
    // It produces less code after transpilation. (#26)
    // eslint-disable-next-line no-useless-constructor
    function List(props) {
      var _this;

      _this = _PureComponent.call(this, props) || this;
      _this._instanceProps = initInstanceProps(_this.props, _assertThisInitialized(_this));
      _this._outerRef = void 0;
      _this._resetIsScrollingTimeoutId = null;
      _this.state = {
        instance: _assertThisInitialized(_this),
        isScrolling: false,
        scrollDirection: 'forward',
        scrollOffset: typeof _this.props.initialScrollOffset === 'number' ? _this.props.initialScrollOffset : 0,
        scrollUpdateWasRequested: false
      };
      _this._callOnItemsRendered = void 0;
      _this._callOnItemsRendered = memoize_one_esm(function (overscanStartIndex, overscanStopIndex, visibleStartIndex, visibleStopIndex) {
        return _this.props.onItemsRendered({
          overscanStartIndex: overscanStartIndex,
          overscanStopIndex: overscanStopIndex,
          visibleStartIndex: visibleStartIndex,
          visibleStopIndex: visibleStopIndex
        });
      });
      _this._callOnScroll = void 0;
      _this._callOnScroll = memoize_one_esm(function (scrollDirection, scrollOffset, scrollUpdateWasRequested) {
        return _this.props.onScroll({
          scrollDirection: scrollDirection,
          scrollOffset: scrollOffset,
          scrollUpdateWasRequested: scrollUpdateWasRequested
        });
      });
      _this._getItemStyle = void 0;

      _this._getItemStyle = function (index) {
        var _this$props = _this.props,
            direction = _this$props.direction,
            itemSize = _this$props.itemSize,
            layout = _this$props.layout;

        var itemStyleCache = _this._getItemStyleCache(shouldResetStyleCacheOnItemSizeChange && itemSize, shouldResetStyleCacheOnItemSizeChange && layout, shouldResetStyleCacheOnItemSizeChange && direction);

        var style;

        if (itemStyleCache.hasOwnProperty(index)) {
          style = itemStyleCache[index];
        } else {
          var _offset = getItemOffset(_this.props, index, _this._instanceProps);

          var size = getItemSize(_this.props, index, _this._instanceProps); // TODO Deprecate direction "horizontal"

          var isHorizontal = direction === 'horizontal' || layout === 'horizontal';
          var isRtl = direction === 'rtl';
          var offsetHorizontal = isHorizontal ? _offset : 0;
          itemStyleCache[index] = style = {
            position: 'absolute',
            left: isRtl ? undefined : offsetHorizontal,
            right: isRtl ? offsetHorizontal : undefined,
            top: !isHorizontal ? _offset : 0,
            height: !isHorizontal ? size : '100%',
            width: isHorizontal ? size : '100%'
          };
        }

        return style;
      };

      _this._getItemStyleCache = void 0;
      _this._getItemStyleCache = memoize_one_esm(function (_, __, ___) {
        return {};
      });

      _this._onScrollHorizontal = function (event) {
        var _event$currentTarget = event.currentTarget,
            clientWidth = _event$currentTarget.clientWidth,
            scrollLeft = _event$currentTarget.scrollLeft,
            scrollWidth = _event$currentTarget.scrollWidth;

        _this.setState(function (prevState) {
          if (prevState.scrollOffset === scrollLeft) {
            // Scroll position may have been updated by cDM/cDU,
            // In which case we don't need to trigger another render,
            // And we don't want to update state.isScrolling.
            return null;
          }

          var direction = _this.props.direction;
          var scrollOffset = scrollLeft;

          if (direction === 'rtl') {
            // TRICKY According to the spec, scrollLeft should be negative for RTL aligned elements.
            // This is not the case for all browsers though (e.g. Chrome reports values as positive, measured relative to the left).
            // It's also easier for this component if we convert offsets to the same format as they would be in for ltr.
            // So the simplest solution is to determine which browser behavior we're dealing with, and convert based on it.
            switch (getRTLOffsetType()) {
              case 'negative':
                scrollOffset = -scrollLeft;
                break;

              case 'positive-descending':
                scrollOffset = scrollWidth - clientWidth - scrollLeft;
                break;
            }
          } // Prevent Safari's elastic scrolling from causing visual shaking when scrolling past bounds.


          scrollOffset = Math.max(0, Math.min(scrollOffset, scrollWidth - clientWidth));
          return {
            isScrolling: true,
            scrollDirection: prevState.scrollOffset < scrollOffset ? 'forward' : 'backward',
            scrollOffset: scrollOffset,
            scrollUpdateWasRequested: false
          };
        }, _this._resetIsScrollingDebounced);
      };

      _this._onScrollVertical = function (event) {
        var _event$currentTarget2 = event.currentTarget,
            clientHeight = _event$currentTarget2.clientHeight,
            scrollHeight = _event$currentTarget2.scrollHeight,
            scrollTop = _event$currentTarget2.scrollTop;

        _this.setState(function (prevState) {
          if (prevState.scrollOffset === scrollTop) {
            // Scroll position may have been updated by cDM/cDU,
            // In which case we don't need to trigger another render,
            // And we don't want to update state.isScrolling.
            return null;
          } // Prevent Safari's elastic scrolling from causing visual shaking when scrolling past bounds.


          var scrollOffset = Math.max(0, Math.min(scrollTop, scrollHeight - clientHeight));
          return {
            isScrolling: true,
            scrollDirection: prevState.scrollOffset < scrollOffset ? 'forward' : 'backward',
            scrollOffset: scrollOffset,
            scrollUpdateWasRequested: false
          };
        }, _this._resetIsScrollingDebounced);
      };

      _this._outerRefSetter = function (ref) {
        var outerRef = _this.props.outerRef;
        _this._outerRef = ref;

        if (typeof outerRef === 'function') {
          outerRef(ref);
        } else if (outerRef != null && typeof outerRef === 'object' && outerRef.hasOwnProperty('current')) {
          outerRef.current = ref;
        }
      };

      _this._resetIsScrollingDebounced = function () {
        if (_this._resetIsScrollingTimeoutId !== null) {
          cancelTimeout(_this._resetIsScrollingTimeoutId);
        }

        _this._resetIsScrollingTimeoutId = requestTimeout(_this._resetIsScrolling, IS_SCROLLING_DEBOUNCE_INTERVAL$1);
      };

      _this._resetIsScrolling = function () {
        _this._resetIsScrollingTimeoutId = null;

        _this.setState({
          isScrolling: false
        }, function () {
          // Clear style cache after state update has been committed.
          // This way we don't break pure sCU for items that don't use isScrolling param.
          _this._getItemStyleCache(-1, null);
        });
      };

      return _this;
    }

    List.getDerivedStateFromProps = function getDerivedStateFromProps(nextProps, prevState) {
      validateSharedProps$1(nextProps, prevState);
      validateProps(nextProps);
      return null;
    };

    var _proto = List.prototype;

    _proto.scrollTo = function scrollTo(scrollOffset) {
      scrollOffset = Math.max(0, scrollOffset);
      this.setState(function (prevState) {
        if (prevState.scrollOffset === scrollOffset) {
          return null;
        }

        return {
          scrollDirection: prevState.scrollOffset < scrollOffset ? 'forward' : 'backward',
          scrollOffset: scrollOffset,
          scrollUpdateWasRequested: true
        };
      }, this._resetIsScrollingDebounced);
    };

    _proto.scrollToItem = function scrollToItem(index, align) {
      if (align === void 0) {
        align = 'auto';
      }

      var _this$props2 = this.props,
          itemCount = _this$props2.itemCount,
          layout = _this$props2.layout;
      var scrollOffset = this.state.scrollOffset;
      index = Math.max(0, Math.min(index, itemCount - 1)); // The scrollbar size should be considered when scrolling an item into view, to ensure it's fully visible.
      // But we only need to account for its size when it's actually visible.
      // This is an edge case for lists; normally they only scroll in the dominant direction.

      var scrollbarSize = 0;

      if (this._outerRef) {
        var outerRef = this._outerRef;

        if (layout === 'vertical') {
          scrollbarSize = outerRef.scrollWidth > outerRef.clientWidth ? getScrollbarSize() : 0;
        } else {
          scrollbarSize = outerRef.scrollHeight > outerRef.clientHeight ? getScrollbarSize() : 0;
        }
      }

      this.scrollTo(getOffsetForIndexAndAlignment(this.props, index, align, scrollOffset, this._instanceProps, scrollbarSize));
    };

    _proto.componentDidMount = function componentDidMount() {
      var _this$props3 = this.props,
          direction = _this$props3.direction,
          initialScrollOffset = _this$props3.initialScrollOffset,
          layout = _this$props3.layout;

      if (typeof initialScrollOffset === 'number' && this._outerRef != null) {
        var outerRef = this._outerRef; // TODO Deprecate direction "horizontal"

        if (direction === 'horizontal' || layout === 'horizontal') {
          outerRef.scrollLeft = initialScrollOffset;
        } else {
          outerRef.scrollTop = initialScrollOffset;
        }
      }

      this._callPropsCallbacks();
    };

    _proto.componentDidUpdate = function componentDidUpdate() {
      var _this$props4 = this.props,
          direction = _this$props4.direction,
          layout = _this$props4.layout;
      var _this$state = this.state,
          scrollOffset = _this$state.scrollOffset,
          scrollUpdateWasRequested = _this$state.scrollUpdateWasRequested;

      if (scrollUpdateWasRequested && this._outerRef != null) {
        var outerRef = this._outerRef; // TODO Deprecate direction "horizontal"

        if (direction === 'horizontal' || layout === 'horizontal') {
          if (direction === 'rtl') {
            // TRICKY According to the spec, scrollLeft should be negative for RTL aligned elements.
            // This is not the case for all browsers though (e.g. Chrome reports values as positive, measured relative to the left).
            // So we need to determine which browser behavior we're dealing with, and mimic it.
            switch (getRTLOffsetType()) {
              case 'negative':
                outerRef.scrollLeft = -scrollOffset;
                break;

              case 'positive-ascending':
                outerRef.scrollLeft = scrollOffset;
                break;

              default:
                var clientWidth = outerRef.clientWidth,
                    scrollWidth = outerRef.scrollWidth;
                outerRef.scrollLeft = scrollWidth - clientWidth - scrollOffset;
                break;
            }
          } else {
            outerRef.scrollLeft = scrollOffset;
          }
        } else {
          outerRef.scrollTop = scrollOffset;
        }
      }

      this._callPropsCallbacks();
    };

    _proto.componentWillUnmount = function componentWillUnmount() {
      if (this._resetIsScrollingTimeoutId !== null) {
        cancelTimeout(this._resetIsScrollingTimeoutId);
      }
    };

    _proto.render = function render() {
      var _this$props5 = this.props,
          children = _this$props5.children,
          className = _this$props5.className,
          direction = _this$props5.direction,
          height = _this$props5.height,
          innerRef = _this$props5.innerRef,
          innerElementType = _this$props5.innerElementType,
          innerTagName = _this$props5.innerTagName,
          itemCount = _this$props5.itemCount,
          itemData = _this$props5.itemData,
          _this$props5$itemKey = _this$props5.itemKey,
          itemKey = _this$props5$itemKey === void 0 ? defaultItemKey$1 : _this$props5$itemKey,
          layout = _this$props5.layout,
          outerElementType = _this$props5.outerElementType,
          outerTagName = _this$props5.outerTagName,
          style = _this$props5.style,
          useIsScrolling = _this$props5.useIsScrolling,
          width = _this$props5.width;
      var isScrolling = this.state.isScrolling; // TODO Deprecate direction "horizontal"

      var isHorizontal = direction === 'horizontal' || layout === 'horizontal';
      var onScroll = isHorizontal ? this._onScrollHorizontal : this._onScrollVertical;

      var _this$_getRangeToRend = this._getRangeToRender(),
          startIndex = _this$_getRangeToRend[0],
          stopIndex = _this$_getRangeToRend[1];

      var items = [];

      if (itemCount > 0) {
        for (var _index = startIndex; _index <= stopIndex; _index++) {
          items.push((0,react.createElement)(children, {
            data: itemData,
            key: itemKey(_index, itemData),
            index: _index,
            isScrolling: useIsScrolling ? isScrolling : undefined,
            style: this._getItemStyle(_index)
          }));
        }
      } // Read this value AFTER items have been created,
      // So their actual sizes (if variable) are taken into consideration.


      var estimatedTotalSize = getEstimatedTotalSize(this.props, this._instanceProps);
      return (0,react.createElement)(outerElementType || outerTagName || 'div', {
        className: className,
        onScroll: onScroll,
        ref: this._outerRefSetter,
        style: _extends({
          position: 'relative',
          height: height,
          width: width,
          overflow: 'auto',
          WebkitOverflowScrolling: 'touch',
          willChange: 'transform',
          direction: direction
        }, style)
      }, (0,react.createElement)(innerElementType || innerTagName || 'div', {
        children: items,
        ref: innerRef,
        style: {
          height: isHorizontal ? '100%' : estimatedTotalSize,
          pointerEvents: isScrolling ? 'none' : undefined,
          width: isHorizontal ? estimatedTotalSize : '100%'
        }
      }));
    };

    _proto._callPropsCallbacks = function _callPropsCallbacks() {
      if (typeof this.props.onItemsRendered === 'function') {
        var itemCount = this.props.itemCount;

        if (itemCount > 0) {
          var _this$_getRangeToRend2 = this._getRangeToRender(),
              _overscanStartIndex = _this$_getRangeToRend2[0],
              _overscanStopIndex = _this$_getRangeToRend2[1],
              _visibleStartIndex = _this$_getRangeToRend2[2],
              _visibleStopIndex = _this$_getRangeToRend2[3];

          this._callOnItemsRendered(_overscanStartIndex, _overscanStopIndex, _visibleStartIndex, _visibleStopIndex);
        }
      }

      if (typeof this.props.onScroll === 'function') {
        var _this$state2 = this.state,
            _scrollDirection = _this$state2.scrollDirection,
            _scrollOffset = _this$state2.scrollOffset,
            _scrollUpdateWasRequested = _this$state2.scrollUpdateWasRequested;

        this._callOnScroll(_scrollDirection, _scrollOffset, _scrollUpdateWasRequested);
      }
    } // Lazily create and cache item styles while scrolling,
    // So that pure component sCU will prevent re-renders.
    // We maintain this cache, and pass a style prop rather than index,
    // So that List can clear cached styles and force item re-render if necessary.
    ;

    _proto._getRangeToRender = function _getRangeToRender() {
      var _this$props6 = this.props,
          itemCount = _this$props6.itemCount,
          overscanCount = _this$props6.overscanCount;
      var _this$state3 = this.state,
          isScrolling = _this$state3.isScrolling,
          scrollDirection = _this$state3.scrollDirection,
          scrollOffset = _this$state3.scrollOffset;

      if (itemCount === 0) {
        return [0, 0, 0, 0];
      }

      var startIndex = getStartIndexForOffset(this.props, scrollOffset, this._instanceProps);
      var stopIndex = getStopIndexForStartIndex(this.props, startIndex, scrollOffset, this._instanceProps); // Overscan by one item in each direction so that tab/focus works.
      // If there isn't at least one extra item, tab loops back around.

      var overscanBackward = !isScrolling || scrollDirection === 'backward' ? Math.max(1, overscanCount) : 1;
      var overscanForward = !isScrolling || scrollDirection === 'forward' ? Math.max(1, overscanCount) : 1;
      return [Math.max(0, startIndex - overscanBackward), Math.max(0, Math.min(itemCount - 1, stopIndex + overscanForward)), startIndex, stopIndex];
    };

    return List;
  }(react.PureComponent), _class.defaultProps = {
    direction: 'ltr',
    itemData: undefined,
    layout: 'vertical',
    overscanCount: 2,
    useIsScrolling: false
  }, _class;
} // NOTE: I considered further wrapping individual items with a pure ListItem component.
// This would avoid ever calling the render function for the same index more than once,
// But it would also add the overhead of a lot of components/fibers.
// I assume people already do this (render function returning a class component),
// So my doing it would just unnecessarily double the wrappers.

var validateSharedProps$1 = function validateSharedProps(_ref2, _ref3) {
  var children = _ref2.children,
      direction = _ref2.direction,
      height = _ref2.height,
      layout = _ref2.layout,
      innerTagName = _ref2.innerTagName,
      outerTagName = _ref2.outerTagName,
      width = _ref2.width;
  var instance = _ref3.instance;

  if (false) { var isHorizontal; }
};

var DEFAULT_ESTIMATED_ITEM_SIZE$1 = 50;

var getItemMetadata$1 = function getItemMetadata(props, index, instanceProps) {
  var _ref = props,
      itemSize = _ref.itemSize;
  var itemMetadataMap = instanceProps.itemMetadataMap,
      lastMeasuredIndex = instanceProps.lastMeasuredIndex;

  if (index > lastMeasuredIndex) {
    var offset = 0;

    if (lastMeasuredIndex >= 0) {
      var itemMetadata = itemMetadataMap[lastMeasuredIndex];
      offset = itemMetadata.offset + itemMetadata.size;
    }

    for (var i = lastMeasuredIndex + 1; i <= index; i++) {
      var size = itemSize(i);
      itemMetadataMap[i] = {
        offset: offset,
        size: size
      };
      offset += size;
    }

    instanceProps.lastMeasuredIndex = index;
  }

  return itemMetadataMap[index];
};

var findNearestItem$1 = function findNearestItem(props, instanceProps, offset) {
  var itemMetadataMap = instanceProps.itemMetadataMap,
      lastMeasuredIndex = instanceProps.lastMeasuredIndex;
  var lastMeasuredItemOffset = lastMeasuredIndex > 0 ? itemMetadataMap[lastMeasuredIndex].offset : 0;

  if (lastMeasuredItemOffset >= offset) {
    // If we've already measured items within this range just use a binary search as it's faster.
    return findNearestItemBinarySearch$1(props, instanceProps, lastMeasuredIndex, 0, offset);
  } else {
    // If we haven't yet measured this high, fallback to an exponential search with an inner binary search.
    // The exponential search avoids pre-computing sizes for the full set of items as a binary search would.
    // The overall complexity for this approach is O(log n).
    return findNearestItemExponentialSearch$1(props, instanceProps, Math.max(0, lastMeasuredIndex), offset);
  }
};

var findNearestItemBinarySearch$1 = function findNearestItemBinarySearch(props, instanceProps, high, low, offset) {
  while (low <= high) {
    var middle = low + Math.floor((high - low) / 2);
    var currentOffset = getItemMetadata$1(props, middle, instanceProps).offset;

    if (currentOffset === offset) {
      return middle;
    } else if (currentOffset < offset) {
      low = middle + 1;
    } else if (currentOffset > offset) {
      high = middle - 1;
    }
  }

  if (low > 0) {
    return low - 1;
  } else {
    return 0;
  }
};

var findNearestItemExponentialSearch$1 = function findNearestItemExponentialSearch(props, instanceProps, index, offset) {
  var itemCount = props.itemCount;
  var interval = 1;

  while (index < itemCount && getItemMetadata$1(props, index, instanceProps).offset < offset) {
    index += interval;
    interval *= 2;
  }

  return findNearestItemBinarySearch$1(props, instanceProps, Math.min(index, itemCount - 1), Math.floor(index / 2), offset);
};

var getEstimatedTotalSize = function getEstimatedTotalSize(_ref2, _ref3) {
  var itemCount = _ref2.itemCount;
  var itemMetadataMap = _ref3.itemMetadataMap,
      estimatedItemSize = _ref3.estimatedItemSize,
      lastMeasuredIndex = _ref3.lastMeasuredIndex;
  var totalSizeOfMeasuredItems = 0; // Edge case check for when the number of items decreases while a scroll is in progress.
  // https://github.com/bvaughn/react-window/pull/138

  if (lastMeasuredIndex >= itemCount) {
    lastMeasuredIndex = itemCount - 1;
  }

  if (lastMeasuredIndex >= 0) {
    var itemMetadata = itemMetadataMap[lastMeasuredIndex];
    totalSizeOfMeasuredItems = itemMetadata.offset + itemMetadata.size;
  }

  var numUnmeasuredItems = itemCount - lastMeasuredIndex - 1;
  var totalSizeOfUnmeasuredItems = numUnmeasuredItems * estimatedItemSize;
  return totalSizeOfMeasuredItems + totalSizeOfUnmeasuredItems;
};

var VariableSizeList = /*#__PURE__*/createListComponent({
  getItemOffset: function getItemOffset(props, index, instanceProps) {
    return getItemMetadata$1(props, index, instanceProps).offset;
  },
  getItemSize: function getItemSize(props, index, instanceProps) {
    return instanceProps.itemMetadataMap[index].size;
  },
  getEstimatedTotalSize: getEstimatedTotalSize,
  getOffsetForIndexAndAlignment: function getOffsetForIndexAndAlignment(props, index, align, scrollOffset, instanceProps, scrollbarSize) {
    var direction = props.direction,
        height = props.height,
        layout = props.layout,
        width = props.width; // TODO Deprecate direction "horizontal"

    var isHorizontal = direction === 'horizontal' || layout === 'horizontal';
    var size = isHorizontal ? width : height;
    var itemMetadata = getItemMetadata$1(props, index, instanceProps); // Get estimated total size after ItemMetadata is computed,
    // To ensure it reflects actual measurements instead of just estimates.

    var estimatedTotalSize = getEstimatedTotalSize(props, instanceProps);
    var maxOffset = Math.max(0, Math.min(estimatedTotalSize - size, itemMetadata.offset));
    var minOffset = Math.max(0, itemMetadata.offset - size + itemMetadata.size + scrollbarSize);

    if (align === 'smart') {
      if (scrollOffset >= minOffset - size && scrollOffset <= maxOffset + size) {
        align = 'auto';
      } else {
        align = 'center';
      }
    }

    switch (align) {
      case 'start':
        return maxOffset;

      case 'end':
        return minOffset;

      case 'center':
        return Math.round(minOffset + (maxOffset - minOffset) / 2);

      case 'auto':
      default:
        if (scrollOffset >= minOffset && scrollOffset <= maxOffset) {
          return scrollOffset;
        } else if (scrollOffset < minOffset) {
          return minOffset;
        } else {
          return maxOffset;
        }

    }
  },
  getStartIndexForOffset: function getStartIndexForOffset(props, offset, instanceProps) {
    return findNearestItem$1(props, instanceProps, offset);
  },
  getStopIndexForStartIndex: function getStopIndexForStartIndex(props, startIndex, scrollOffset, instanceProps) {
    var direction = props.direction,
        height = props.height,
        itemCount = props.itemCount,
        layout = props.layout,
        width = props.width; // TODO Deprecate direction "horizontal"

    var isHorizontal = direction === 'horizontal' || layout === 'horizontal';
    var size = isHorizontal ? width : height;
    var itemMetadata = getItemMetadata$1(props, startIndex, instanceProps);
    var maxOffset = scrollOffset + size;
    var offset = itemMetadata.offset + itemMetadata.size;
    var stopIndex = startIndex;

    while (stopIndex < itemCount - 1 && offset < maxOffset) {
      stopIndex++;
      offset += getItemMetadata$1(props, stopIndex, instanceProps).size;
    }

    return stopIndex;
  },
  initInstanceProps: function initInstanceProps(props, instance) {
    var _ref4 = props,
        estimatedItemSize = _ref4.estimatedItemSize;
    var instanceProps = {
      itemMetadataMap: {},
      estimatedItemSize: estimatedItemSize || DEFAULT_ESTIMATED_ITEM_SIZE$1,
      lastMeasuredIndex: -1
    };

    instance.resetAfterIndex = function (index, shouldForceUpdate) {
      if (shouldForceUpdate === void 0) {
        shouldForceUpdate = true;
      }

      instanceProps.lastMeasuredIndex = Math.min(instanceProps.lastMeasuredIndex, index - 1); // We could potentially optimize further by only evicting styles after this index,
      // But since styles are only cached while scrolling is in progress-
      // It seems an unnecessary optimization.
      // It's unlikely that resetAfterIndex() will be called while a user is scrolling.

      instance._getItemStyleCache(-1);

      if (shouldForceUpdate) {
        instance.forceUpdate();
      }
    };

    return instanceProps;
  },
  shouldResetStyleCacheOnItemSizeChange: false,
  validateProps: function validateProps(_ref5) {
    var itemSize = _ref5.itemSize;

    if (false) {}
  }
});

var FixedSizeGrid = /*#__PURE__*/createGridComponent({
  getColumnOffset: function getColumnOffset(_ref, index) {
    var columnWidth = _ref.columnWidth;
    return index * columnWidth;
  },
  getColumnWidth: function getColumnWidth(_ref2, index) {
    var columnWidth = _ref2.columnWidth;
    return columnWidth;
  },
  getRowOffset: function getRowOffset(_ref3, index) {
    var rowHeight = _ref3.rowHeight;
    return index * rowHeight;
  },
  getRowHeight: function getRowHeight(_ref4, index) {
    var rowHeight = _ref4.rowHeight;
    return rowHeight;
  },
  getEstimatedTotalHeight: function getEstimatedTotalHeight(_ref5) {
    var rowCount = _ref5.rowCount,
        rowHeight = _ref5.rowHeight;
    return rowHeight * rowCount;
  },
  getEstimatedTotalWidth: function getEstimatedTotalWidth(_ref6) {
    var columnCount = _ref6.columnCount,
        columnWidth = _ref6.columnWidth;
    return columnWidth * columnCount;
  },
  getOffsetForColumnAndAlignment: function getOffsetForColumnAndAlignment(_ref7, columnIndex, align, scrollLeft, instanceProps, scrollbarSize) {
    var columnCount = _ref7.columnCount,
        columnWidth = _ref7.columnWidth,
        width = _ref7.width;
    var lastColumnOffset = Math.max(0, columnCount * columnWidth - width);
    var maxOffset = Math.min(lastColumnOffset, columnIndex * columnWidth);
    var minOffset = Math.max(0, columnIndex * columnWidth - width + scrollbarSize + columnWidth);

    if (align === 'smart') {
      if (scrollLeft >= minOffset - width && scrollLeft <= maxOffset + width) {
        align = 'auto';
      } else {
        align = 'center';
      }
    }

    switch (align) {
      case 'start':
        return maxOffset;

      case 'end':
        return minOffset;

      case 'center':
        // "Centered" offset is usually the average of the min and max.
        // But near the edges of the list, this doesn't hold true.
        var middleOffset = Math.round(minOffset + (maxOffset - minOffset) / 2);

        if (middleOffset < Math.ceil(width / 2)) {
          return 0; // near the beginning
        } else if (middleOffset > lastColumnOffset + Math.floor(width / 2)) {
          return lastColumnOffset; // near the end
        } else {
          return middleOffset;
        }

      case 'auto':
      default:
        if (scrollLeft >= minOffset && scrollLeft <= maxOffset) {
          return scrollLeft;
        } else if (minOffset > maxOffset) {
          // Because we only take into account the scrollbar size when calculating minOffset
          // this value can be larger than maxOffset when at the end of the list
          return minOffset;
        } else if (scrollLeft < minOffset) {
          return minOffset;
        } else {
          return maxOffset;
        }

    }
  },
  getOffsetForRowAndAlignment: function getOffsetForRowAndAlignment(_ref8, rowIndex, align, scrollTop, instanceProps, scrollbarSize) {
    var rowHeight = _ref8.rowHeight,
        height = _ref8.height,
        rowCount = _ref8.rowCount;
    var lastRowOffset = Math.max(0, rowCount * rowHeight - height);
    var maxOffset = Math.min(lastRowOffset, rowIndex * rowHeight);
    var minOffset = Math.max(0, rowIndex * rowHeight - height + scrollbarSize + rowHeight);

    if (align === 'smart') {
      if (scrollTop >= minOffset - height && scrollTop <= maxOffset + height) {
        align = 'auto';
      } else {
        align = 'center';
      }
    }

    switch (align) {
      case 'start':
        return maxOffset;

      case 'end':
        return minOffset;

      case 'center':
        // "Centered" offset is usually the average of the min and max.
        // But near the edges of the list, this doesn't hold true.
        var middleOffset = Math.round(minOffset + (maxOffset - minOffset) / 2);

        if (middleOffset < Math.ceil(height / 2)) {
          return 0; // near the beginning
        } else if (middleOffset > lastRowOffset + Math.floor(height / 2)) {
          return lastRowOffset; // near the end
        } else {
          return middleOffset;
        }

      case 'auto':
      default:
        if (scrollTop >= minOffset && scrollTop <= maxOffset) {
          return scrollTop;
        } else if (minOffset > maxOffset) {
          // Because we only take into account the scrollbar size when calculating minOffset
          // this value can be larger than maxOffset when at the end of the list
          return minOffset;
        } else if (scrollTop < minOffset) {
          return minOffset;
        } else {
          return maxOffset;
        }

    }
  },
  getColumnStartIndexForOffset: function getColumnStartIndexForOffset(_ref9, scrollLeft) {
    var columnWidth = _ref9.columnWidth,
        columnCount = _ref9.columnCount;
    return Math.max(0, Math.min(columnCount - 1, Math.floor(scrollLeft / columnWidth)));
  },
  getColumnStopIndexForStartIndex: function getColumnStopIndexForStartIndex(_ref10, startIndex, scrollLeft) {
    var columnWidth = _ref10.columnWidth,
        columnCount = _ref10.columnCount,
        width = _ref10.width;
    var left = startIndex * columnWidth;
    var numVisibleColumns = Math.ceil((width + scrollLeft - left) / columnWidth);
    return Math.max(0, Math.min(columnCount - 1, startIndex + numVisibleColumns - 1 // -1 is because stop index is inclusive
    ));
  },
  getRowStartIndexForOffset: function getRowStartIndexForOffset(_ref11, scrollTop) {
    var rowHeight = _ref11.rowHeight,
        rowCount = _ref11.rowCount;
    return Math.max(0, Math.min(rowCount - 1, Math.floor(scrollTop / rowHeight)));
  },
  getRowStopIndexForStartIndex: function getRowStopIndexForStartIndex(_ref12, startIndex, scrollTop) {
    var rowHeight = _ref12.rowHeight,
        rowCount = _ref12.rowCount,
        height = _ref12.height;
    var top = startIndex * rowHeight;
    var numVisibleRows = Math.ceil((height + scrollTop - top) / rowHeight);
    return Math.max(0, Math.min(rowCount - 1, startIndex + numVisibleRows - 1 // -1 is because stop index is inclusive
    ));
  },
  initInstanceProps: function initInstanceProps(props) {// Noop
  },
  shouldResetStyleCacheOnItemSizeChange: true,
  validateProps: function validateProps(_ref13) {
    var columnWidth = _ref13.columnWidth,
        rowHeight = _ref13.rowHeight;

    if (false) {}
  }
});

var FixedSizeList = /*#__PURE__*/createListComponent({
  getItemOffset: function getItemOffset(_ref, index) {
    var itemSize = _ref.itemSize;
    return index * itemSize;
  },
  getItemSize: function getItemSize(_ref2, index) {
    var itemSize = _ref2.itemSize;
    return itemSize;
  },
  getEstimatedTotalSize: function getEstimatedTotalSize(_ref3) {
    var itemCount = _ref3.itemCount,
        itemSize = _ref3.itemSize;
    return itemSize * itemCount;
  },
  getOffsetForIndexAndAlignment: function getOffsetForIndexAndAlignment(_ref4, index, align, scrollOffset, instanceProps, scrollbarSize) {
    var direction = _ref4.direction,
        height = _ref4.height,
        itemCount = _ref4.itemCount,
        itemSize = _ref4.itemSize,
        layout = _ref4.layout,
        width = _ref4.width;
    // TODO Deprecate direction "horizontal"
    var isHorizontal = direction === 'horizontal' || layout === 'horizontal';
    var size = isHorizontal ? width : height;
    var lastItemOffset = Math.max(0, itemCount * itemSize - size);
    var maxOffset = Math.min(lastItemOffset, index * itemSize);
    var minOffset = Math.max(0, index * itemSize - size + itemSize + scrollbarSize);

    if (align === 'smart') {
      if (scrollOffset >= minOffset - size && scrollOffset <= maxOffset + size) {
        align = 'auto';
      } else {
        align = 'center';
      }
    }

    switch (align) {
      case 'start':
        return maxOffset;

      case 'end':
        return minOffset;

      case 'center':
        {
          // "Centered" offset is usually the average of the min and max.
          // But near the edges of the list, this doesn't hold true.
          var middleOffset = Math.round(minOffset + (maxOffset - minOffset) / 2);

          if (middleOffset < Math.ceil(size / 2)) {
            return 0; // near the beginning
          } else if (middleOffset > lastItemOffset + Math.floor(size / 2)) {
            return lastItemOffset; // near the end
          } else {
            return middleOffset;
          }
        }

      case 'auto':
      default:
        if (scrollOffset >= minOffset && scrollOffset <= maxOffset) {
          return scrollOffset;
        } else if (scrollOffset < minOffset) {
          return minOffset;
        } else {
          return maxOffset;
        }

    }
  },
  getStartIndexForOffset: function getStartIndexForOffset(_ref5, offset) {
    var itemCount = _ref5.itemCount,
        itemSize = _ref5.itemSize;
    return Math.max(0, Math.min(itemCount - 1, Math.floor(offset / itemSize)));
  },
  getStopIndexForStartIndex: function getStopIndexForStartIndex(_ref6, startIndex, scrollOffset) {
    var direction = _ref6.direction,
        height = _ref6.height,
        itemCount = _ref6.itemCount,
        itemSize = _ref6.itemSize,
        layout = _ref6.layout,
        width = _ref6.width;
    // TODO Deprecate direction "horizontal"
    var isHorizontal = direction === 'horizontal' || layout === 'horizontal';
    var offset = startIndex * itemSize;
    var size = isHorizontal ? width : height;
    var numVisibleItems = Math.ceil((size + scrollOffset - offset) / itemSize);
    return Math.max(0, Math.min(itemCount - 1, startIndex + numVisibleItems - 1 // -1 is because stop index is inclusive
    ));
  },
  initInstanceProps: function initInstanceProps(props) {// Noop
  },
  shouldResetStyleCacheOnItemSizeChange: true,
  validateProps: function validateProps(_ref7) {
    var itemSize = _ref7.itemSize;

    if (false) {}
  }
});

// Pulled from react-compat
// https://github.com/developit/preact-compat/blob/7c5de00e7c85e2ffd011bf3af02899b63f699d3a/src/index.js#L349
function shallowDiffers(prev, next) {
  for (var attribute in prev) {
    if (!(attribute in next)) {
      return true;
    }
  }

  for (var _attribute in next) {
    if (prev[_attribute] !== next[_attribute]) {
      return true;
    }
  }

  return false;
}

var _excluded = (/* unused pure expression or super */ null && (["style"])),
    _excluded2 = (/* unused pure expression or super */ null && (["style"]));
// It knows to compare individual style props and ignore the wrapper object.
// See https://reactjs.org/docs/react-api.html#reactmemo

function areEqual(prevProps, nextProps) {
  var prevStyle = prevProps.style,
      prevRest = _objectWithoutPropertiesLoose(prevProps, _excluded);

  var nextStyle = nextProps.style,
      nextRest = _objectWithoutPropertiesLoose(nextProps, _excluded2);

  return !shallowDiffers(prevStyle, nextStyle) && !shallowDiffers(prevRest, nextRest);
}

// It knows to compare individual style props and ignore the wrapper object.
// See https://reactjs.org/docs/react-component.html#shouldcomponentupdate

function shouldComponentUpdate(nextProps, nextState) {
  return !areEqual(this.props, nextProps) || shallowDiffers(this.state, nextState);
}


//# sourceMappingURL=index.esm.js.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/cursor.js


function Cursor() {
    var _a, _b;
    const tree = useTreeApi();
    const state = useDndContext();
    const cursor = state.cursor;
    if (!cursor || cursor.type !== "line")
        return null;
    const indent = tree.indent;
    const top = tree.rowHeight * cursor.index +
        ((_b = (_a = tree.props.padding) !== null && _a !== void 0 ? _a : tree.props.paddingTop) !== null && _b !== void 0 ? _b : 0);
    const left = indent * cursor.level;
    const Cursor = tree.renderCursor;
    return (0,react_jsx_runtime.jsx)(Cursor, { top, left, indent });
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/list-outer-element.js
var __rest = (undefined && undefined.__rest) || function (s, e) {
    var t = {};
    for (var p in s) if (Object.prototype.hasOwnProperty.call(s, p) && e.indexOf(p) < 0)
        t[p] = s[p];
    if (s != null && typeof Object.getOwnPropertySymbols === "function")
        for (var i = 0, p = Object.getOwnPropertySymbols(s); i < p.length; i++) {
            if (e.indexOf(p[i]) < 0 && Object.prototype.propertyIsEnumerable.call(s, p[i]))
                t[p[i]] = s[p[i]];
        }
    return t;
};




const ListOuterElement = (0,react.forwardRef)(function Outer(props, ref) {
    const { children } = props, rest = __rest(props, ["children"]);
    const tree = useTreeApi();
    return ((0,react_jsx_runtime.jsxs)("div", Object.assign({ 
        // @ts-ignore
        ref: ref }, rest, { onClick: (e) => {
            if (e.currentTarget === e.target)
                tree.deselectAll();
        }, children: [(0,react_jsx_runtime.jsx)(DropContainer, {}), children] })));
});
const DropContainer = () => {
    const tree = useTreeApi();
    return ((0,react_jsx_runtime.jsx)("div", { style: {
            height: tree.visibleNodes.length * tree.rowHeight,
            width: "100%",
            position: "absolute",
            left: "0",
            right: "0",
        }, children: (0,react_jsx_runtime.jsx)(Cursor, {}) }));
};

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/list-inner-element.js
var list_inner_element_rest = (undefined && undefined.__rest) || function (s, e) {
    var t = {};
    for (var p in s) if (Object.prototype.hasOwnProperty.call(s, p) && e.indexOf(p) < 0)
        t[p] = s[p];
    if (s != null && typeof Object.getOwnPropertySymbols === "function")
        for (var i = 0, p = Object.getOwnPropertySymbols(s); i < p.length; i++) {
            if (e.indexOf(p[i]) < 0 && Object.prototype.propertyIsEnumerable.call(s, p[i]))
                t[p[i]] = s[p[i]];
        }
    return t;
};



const ListInnerElement = (0,react.forwardRef)(function InnerElement(_a, ref) {
    var _b, _c, _d, _e;
    var { style } = _a, rest = list_inner_element_rest(_a, ["style"]);
    const tree = useTreeApi();
    const paddingTop = (_c = (_b = tree.props.padding) !== null && _b !== void 0 ? _b : tree.props.paddingTop) !== null && _c !== void 0 ? _c : 0;
    const paddingBottom = (_e = (_d = tree.props.padding) !== null && _d !== void 0 ? _d : tree.props.paddingBottom) !== null && _e !== void 0 ? _e : 0;
    return ((0,react_jsx_runtime.jsx)("div", Object.assign({ ref: ref, style: Object.assign(Object.assign({}, style), { height: `${parseFloat(style.height) + paddingTop + paddingBottom}px` }) }, rest)));
});

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/internals/registration.js
function registerTarget(type, target, manager) {
  var registry = manager.getRegistry();
  var targetId = registry.addTarget(type, target);
  return [targetId, function () {
    return registry.removeTarget(targetId);
  }];
}
function registerSource(type, source, manager) {
  var registry = manager.getRegistry();
  var sourceId = registry.addSource(type, source);
  return [sourceId, function () {
    return registry.removeSource(sourceId);
  }];
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useIsomorphicLayoutEffect.js
 // suppress the useLayoutEffect warning on server side.

var useIsomorphicLayoutEffect = typeof window !== 'undefined' ? react.useLayoutEffect : react.useEffect;
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrag/DragSourceImpl.js
function _typeof(obj) { "@babel/helpers - typeof"; if (typeof Symbol === "function" && typeof Symbol.iterator === "symbol") { _typeof = function _typeof(obj) { return typeof obj; }; } else { _typeof = function _typeof(obj) { return obj && typeof Symbol === "function" && obj.constructor === Symbol && obj !== Symbol.prototype ? "symbol" : typeof obj; }; } return _typeof(obj); }

function _classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function _defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function _createClass(Constructor, protoProps, staticProps) { if (protoProps) _defineProperties(Constructor.prototype, protoProps); if (staticProps) _defineProperties(Constructor, staticProps); return Constructor; }

function _defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }

var DragSourceImpl = /*#__PURE__*/function () {
  function DragSourceImpl(spec, monitor, connector) {
    _classCallCheck(this, DragSourceImpl);

    _defineProperty(this, "spec", void 0);

    _defineProperty(this, "monitor", void 0);

    _defineProperty(this, "connector", void 0);

    this.spec = spec;
    this.monitor = monitor;
    this.connector = connector;
  }

  _createClass(DragSourceImpl, [{
    key: "beginDrag",
    value: function beginDrag() {
      var _result;

      var spec = this.spec;
      var monitor = this.monitor;
      var result = null;

      if (_typeof(spec.item) === 'object') {
        result = spec.item;
      } else if (typeof spec.item === 'function') {
        result = spec.item(monitor);
      } else {
        result = {};
      }

      return (_result = result) !== null && _result !== void 0 ? _result : null;
    }
  }, {
    key: "canDrag",
    value: function canDrag() {
      var spec = this.spec;
      var monitor = this.monitor;

      if (typeof spec.canDrag === 'boolean') {
        return spec.canDrag;
      } else if (typeof spec.canDrag === 'function') {
        return spec.canDrag(monitor);
      } else {
        return true;
      }
    }
  }, {
    key: "isDragging",
    value: function isDragging(globalMonitor, target) {
      var spec = this.spec;
      var monitor = this.monitor;
      var isDragging = spec.isDragging;
      return isDragging ? isDragging(monitor) : target === globalMonitor.getSourceId();
    }
  }, {
    key: "endDrag",
    value: function endDrag() {
      var spec = this.spec;
      var monitor = this.monitor;
      var connector = this.connector;
      var end = spec.end;

      if (end) {
        end(monitor.getItem(), monitor);
      }

      connector.reconnect();
    }
  }]);

  return DragSourceImpl;
}();
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrag/useDragSource.js


function useDragSource(spec, monitor, connector) {
  var handler = (0,react.useMemo)(function () {
    return new DragSourceImpl(spec, monitor, connector);
  }, [monitor, connector]);
  (0,react.useEffect)(function () {
    handler.spec = spec;
  }, [spec]);
  return handler;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@react-dnd+invariant@2.0.0/node_modules/@react-dnd/invariant/dist/invariant.esm.js
/**
 * Use invariant() to assert state which your program assumes to be true.
 *
 * Provide sprintf-style format (only %s is supported) and arguments
 * to provide information about what broke and what you were
 * expecting.
 *
 * The invariant message will be stripped in production, but the invariant
 * will remain to ensure logic does not differ in production.
 */
function invariant(condition, format) {
  for (var _len = arguments.length, args = new Array(_len > 2 ? _len - 2 : 0), _key = 2; _key < _len; _key++) {
    args[_key - 2] = arguments[_key];
  }

  if (false) {}

  if (!condition) {
    var error;

    if (format === undefined) {
      error = new Error('Minified exception occurred; use the non-minified dev environment ' + 'for the full error message and additional helpful warnings.');
    } else {
      var argIndex = 0;
      error = new Error(format.replace(/%s/g, function () {
        return args[argIndex++];
      }));
      error.name = 'Invariant Violation';
    }

    error.framesToPop = 1; // we don't care about invariant's own frame

    throw error;
  }
}


//# sourceMappingURL=invariant.esm.js.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/core/DndContext.js

/**
 * Create the React Context
 */

var DndContext_DndContext = (0,react.createContext)({
  dragDropManager: undefined
});
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDragDropManager.js



/**
 * A hook to retrieve the DragDropManager from Context
 */

function useDragDropManager() {
  var _useContext = (0,react.useContext)(DndContext_DndContext),
      dragDropManager = _useContext.dragDropManager;

  invariant(dragDropManager != null, 'Expected drag drop context');
  return dragDropManager;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrag/useDragType.js


function useDragType(spec) {
  return (0,react.useMemo)(function () {
    var result = spec.type;
    invariant(result != null, 'spec.type must be defined');
    return result;
  }, [spec]);
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrag/useRegisteredDragSource.js
function _slicedToArray(arr, i) { return _arrayWithHoles(arr) || _iterableToArrayLimit(arr, i) || _unsupportedIterableToArray(arr, i) || _nonIterableRest(); }

function _nonIterableRest() { throw new TypeError("Invalid attempt to destructure non-iterable instance.\nIn order to be iterable, non-array objects must have a [Symbol.iterator]() method."); }

function _unsupportedIterableToArray(o, minLen) { if (!o) return; if (typeof o === "string") return _arrayLikeToArray(o, minLen); var n = Object.prototype.toString.call(o).slice(8, -1); if (n === "Object" && o.constructor) n = o.constructor.name; if (n === "Map" || n === "Set") return Array.from(o); if (n === "Arguments" || /^(?:Ui|I)nt(?:8|16|32)(?:Clamped)?Array$/.test(n)) return _arrayLikeToArray(o, minLen); }

function _arrayLikeToArray(arr, len) { if (len == null || len > arr.length) len = arr.length; for (var i = 0, arr2 = new Array(len); i < len; i++) { arr2[i] = arr[i]; } return arr2; }

function _iterableToArrayLimit(arr, i) { var _i = arr == null ? null : typeof Symbol !== "undefined" && arr[Symbol.iterator] || arr["@@iterator"]; if (_i == null) return; var _arr = []; var _n = true; var _d = false; var _s, _e; try { for (_i = _i.call(arr); !(_n = (_s = _i.next()).done); _n = true) { _arr.push(_s.value); if (i && _arr.length === i) break; } } catch (err) { _d = true; _e = err; } finally { try { if (!_n && _i["return"] != null) _i["return"](); } finally { if (_d) throw _e; } } return _arr; }

function _arrayWithHoles(arr) { if (Array.isArray(arr)) return arr; }






function useRegisteredDragSource(spec, monitor, connector) {
  var manager = useDragDropManager();
  var handler = useDragSource(spec, monitor, connector);
  var itemType = useDragType(spec);
  useIsomorphicLayoutEffect(function registerDragSource() {
    if (itemType != null) {
      var _registerSource = registerSource(itemType, handler, manager),
          _registerSource2 = _slicedToArray(_registerSource, 2),
          handlerId = _registerSource2[0],
          unregister = _registerSource2[1];

      monitor.receiveHandlerId(handlerId);
      connector.receiveHandlerId(handlerId);
      return unregister;
    }
  }, [manager, monitor, connector, handler, itemType]);
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useOptionalFactory.js
function _toConsumableArray(arr) { return _arrayWithoutHoles(arr) || _iterableToArray(arr) || useOptionalFactory_unsupportedIterableToArray(arr) || _nonIterableSpread(); }

function _nonIterableSpread() { throw new TypeError("Invalid attempt to spread non-iterable instance.\nIn order to be iterable, non-array objects must have a [Symbol.iterator]() method."); }

function useOptionalFactory_unsupportedIterableToArray(o, minLen) { if (!o) return; if (typeof o === "string") return useOptionalFactory_arrayLikeToArray(o, minLen); var n = Object.prototype.toString.call(o).slice(8, -1); if (n === "Object" && o.constructor) n = o.constructor.name; if (n === "Map" || n === "Set") return Array.from(o); if (n === "Arguments" || /^(?:Ui|I)nt(?:8|16|32)(?:Clamped)?Array$/.test(n)) return useOptionalFactory_arrayLikeToArray(o, minLen); }

function _iterableToArray(iter) { if (typeof Symbol !== "undefined" && iter[Symbol.iterator] != null || iter["@@iterator"] != null) return Array.from(iter); }

function _arrayWithoutHoles(arr) { if (Array.isArray(arr)) return useOptionalFactory_arrayLikeToArray(arr); }

function useOptionalFactory_arrayLikeToArray(arr, len) { if (len == null || len > arr.length) len = arr.length; for (var i = 0, arr2 = new Array(len); i < len; i++) { arr2[i] = arr[i]; } return arr2; }


function useOptionalFactory(arg, deps) {
  var memoDeps = _toConsumableArray(deps || []);

  if (deps == null && typeof arg !== 'function') {
    memoDeps.push(arg);
  }

  return (0,react.useMemo)(function () {
    return typeof arg === 'function' ? arg() : arg;
  }, memoDeps);
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/internals/DragSourceMonitorImpl.js
function DragSourceMonitorImpl_classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function DragSourceMonitorImpl_defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function DragSourceMonitorImpl_createClass(Constructor, protoProps, staticProps) { if (protoProps) DragSourceMonitorImpl_defineProperties(Constructor.prototype, protoProps); if (staticProps) DragSourceMonitorImpl_defineProperties(Constructor, staticProps); return Constructor; }

function DragSourceMonitorImpl_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }


var isCallingCanDrag = false;
var isCallingIsDragging = false;
var DragSourceMonitorImpl = /*#__PURE__*/function () {
  function DragSourceMonitorImpl(manager) {
    DragSourceMonitorImpl_classCallCheck(this, DragSourceMonitorImpl);

    DragSourceMonitorImpl_defineProperty(this, "internalMonitor", void 0);

    DragSourceMonitorImpl_defineProperty(this, "sourceId", null);

    this.internalMonitor = manager.getMonitor();
  }

  DragSourceMonitorImpl_createClass(DragSourceMonitorImpl, [{
    key: "receiveHandlerId",
    value: function receiveHandlerId(sourceId) {
      this.sourceId = sourceId;
    }
  }, {
    key: "getHandlerId",
    value: function getHandlerId() {
      return this.sourceId;
    }
  }, {
    key: "canDrag",
    value: function canDrag() {
      invariant(!isCallingCanDrag, 'You may not call monitor.canDrag() inside your canDrag() implementation. ' + 'Read more: http://react-dnd.github.io/react-dnd/docs/api/drag-source-monitor');

      try {
        isCallingCanDrag = true;
        return this.internalMonitor.canDragSource(this.sourceId);
      } finally {
        isCallingCanDrag = false;
      }
    }
  }, {
    key: "isDragging",
    value: function isDragging() {
      if (!this.sourceId) {
        return false;
      }

      invariant(!isCallingIsDragging, 'You may not call monitor.isDragging() inside your isDragging() implementation. ' + 'Read more: http://react-dnd.github.io/react-dnd/docs/api/drag-source-monitor');

      try {
        isCallingIsDragging = true;
        return this.internalMonitor.isDraggingSource(this.sourceId);
      } finally {
        isCallingIsDragging = false;
      }
    }
  }, {
    key: "subscribeToStateChange",
    value: function subscribeToStateChange(listener, options) {
      return this.internalMonitor.subscribeToStateChange(listener, options);
    }
  }, {
    key: "isDraggingSource",
    value: function isDraggingSource(sourceId) {
      return this.internalMonitor.isDraggingSource(sourceId);
    }
  }, {
    key: "isOverTarget",
    value: function isOverTarget(targetId, options) {
      return this.internalMonitor.isOverTarget(targetId, options);
    }
  }, {
    key: "getTargetIds",
    value: function getTargetIds() {
      return this.internalMonitor.getTargetIds();
    }
  }, {
    key: "isSourcePublic",
    value: function isSourcePublic() {
      return this.internalMonitor.isSourcePublic();
    }
  }, {
    key: "getSourceId",
    value: function getSourceId() {
      return this.internalMonitor.getSourceId();
    }
  }, {
    key: "subscribeToOffsetChange",
    value: function subscribeToOffsetChange(listener) {
      return this.internalMonitor.subscribeToOffsetChange(listener);
    }
  }, {
    key: "canDragSource",
    value: function canDragSource(sourceId) {
      return this.internalMonitor.canDragSource(sourceId);
    }
  }, {
    key: "canDropOnTarget",
    value: function canDropOnTarget(targetId) {
      return this.internalMonitor.canDropOnTarget(targetId);
    }
  }, {
    key: "getItemType",
    value: function getItemType() {
      return this.internalMonitor.getItemType();
    }
  }, {
    key: "getItem",
    value: function getItem() {
      return this.internalMonitor.getItem();
    }
  }, {
    key: "getDropResult",
    value: function getDropResult() {
      return this.internalMonitor.getDropResult();
    }
  }, {
    key: "didDrop",
    value: function didDrop() {
      return this.internalMonitor.didDrop();
    }
  }, {
    key: "getInitialClientOffset",
    value: function getInitialClientOffset() {
      return this.internalMonitor.getInitialClientOffset();
    }
  }, {
    key: "getInitialSourceClientOffset",
    value: function getInitialSourceClientOffset() {
      return this.internalMonitor.getInitialSourceClientOffset();
    }
  }, {
    key: "getSourceClientOffset",
    value: function getSourceClientOffset() {
      return this.internalMonitor.getSourceClientOffset();
    }
  }, {
    key: "getClientOffset",
    value: function getClientOffset() {
      return this.internalMonitor.getClientOffset();
    }
  }, {
    key: "getDifferenceFromInitialOffset",
    value: function getDifferenceFromInitialOffset() {
      return this.internalMonitor.getDifferenceFromInitialOffset();
    }
  }]);

  return DragSourceMonitorImpl;
}();
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrag/useDragSourceMonitor.js



function useDragSourceMonitor() {
  var manager = useDragDropManager();
  return (0,react.useMemo)(function () {
    return new DragSourceMonitorImpl(manager);
  }, [manager]);
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/internals/wrapConnectorHooks.js



function throwIfCompositeComponentElement(element) {
  // Custom components can no longer be wrapped directly in React DnD 2.0
  // so that we don't need to depend on findDOMNode() from react-dom.
  if (typeof element.type === 'string') {
    return;
  }

  var displayName = element.type.displayName || element.type.name || 'the component';
  throw new Error('Only native element nodes can now be passed to React DnD connectors.' + "You can either wrap ".concat(displayName, " into a <div>, or turn it into a ") + 'drag source or a drop target itself.');
}

function wrapHookToRecognizeElement(hook) {
  return function () {
    var elementOrNode = arguments.length > 0 && arguments[0] !== undefined ? arguments[0] : null;
    var options = arguments.length > 1 && arguments[1] !== undefined ? arguments[1] : null;

    // When passed a node, call the hook straight away.
    if (!(0,react.isValidElement)(elementOrNode)) {
      var node = elementOrNode;
      hook(node, options); // return the node so it can be chained (e.g. when within callback refs
      // <div ref={node => connectDragSource(connectDropTarget(node))}/>

      return node;
    } // If passed a ReactElement, clone it and attach this function as a ref.
    // This helps us achieve a neat API where user doesn't even know that refs
    // are being used under the hood.


    var element = elementOrNode;
    throwIfCompositeComponentElement(element); // When no options are passed, use the hook directly

    var ref = options ? function (node) {
      return hook(node, options);
    } : hook;
    return cloneWithRef(element, ref);
  };
}

function wrapConnectorHooks(hooks) {
  var wrappedHooks = {};
  Object.keys(hooks).forEach(function (key) {
    var hook = hooks[key]; // ref objects should be passed straight through without wrapping

    if (key.endsWith('Ref')) {
      wrappedHooks[key] = hooks[key];
    } else {
      var wrappedHook = wrapHookToRecognizeElement(hook);

      wrappedHooks[key] = function () {
        return wrappedHook;
      };
    }
  });
  return wrappedHooks;
}

function setRef(ref, node) {
  if (typeof ref === 'function') {
    ref(node);
  } else {
    ref.current = node;
  }
}

function cloneWithRef(element, newRef) {
  var previousRef = element.ref;
  invariant(typeof previousRef !== 'string', 'Cannot connect React DnD to an element with an existing string ref. ' + 'Please convert it to use a callback ref instead, or wrap it into a <span> or <div>. ' + 'Read more: https://reactjs.org/docs/refs-and-the-dom.html#callback-refs');

  if (!previousRef) {
    // When there is no ref on the element, use the new ref directly
    return (0,react.cloneElement)(element, {
      ref: newRef
    });
  } else {
    return (0,react.cloneElement)(element, {
      ref: function ref(node) {
        setRef(previousRef, node);
        setRef(newRef, node);
      }
    });
  }
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/internals/isRef.js
function isRef_typeof(obj) { "@babel/helpers - typeof"; if (typeof Symbol === "function" && typeof Symbol.iterator === "symbol") { isRef_typeof = function _typeof(obj) { return typeof obj; }; } else { isRef_typeof = function _typeof(obj) { return obj && typeof Symbol === "function" && obj.constructor === Symbol && obj !== Symbol.prototype ? "symbol" : typeof obj; }; } return isRef_typeof(obj); }

function isRef(obj) {
  return (// eslint-disable-next-line no-prototype-builtins
    obj !== null && isRef_typeof(obj) === 'object' && Object.prototype.hasOwnProperty.call(obj, 'current')
  );
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@react-dnd+shallowequal@2.0.0/node_modules/@react-dnd/shallowequal/dist/shallowequal.esm.js
function shallowEqual(objA, objB, compare, compareContext) {
  var compareResult = compare ? compare.call(compareContext, objA, objB) : void 0;

  if (compareResult !== void 0) {
    return !!compareResult;
  }

  if (objA === objB) {
    return true;
  }

  if (typeof objA !== 'object' || !objA || typeof objB !== 'object' || !objB) {
    return false;
  }

  var keysA = Object.keys(objA);
  var keysB = Object.keys(objB);

  if (keysA.length !== keysB.length) {
    return false;
  }

  var bHasOwnProperty = Object.prototype.hasOwnProperty.bind(objB); // Test for A's keys different from B.

  for (var idx = 0; idx < keysA.length; idx++) {
    var key = keysA[idx];

    if (!bHasOwnProperty(key)) {
      return false;
    }

    var valueA = objA[key];
    var valueB = objB[key];
    compareResult = compare ? compare.call(compareContext, valueA, valueB, key) : void 0;

    if (compareResult === false || compareResult === void 0 && valueA !== valueB) {
      return false;
    }
  }

  return true;
}


//# sourceMappingURL=shallowequal.esm.js.map

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/internals/SourceConnector.js
function SourceConnector_classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function SourceConnector_defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function SourceConnector_createClass(Constructor, protoProps, staticProps) { if (protoProps) SourceConnector_defineProperties(Constructor.prototype, protoProps); if (staticProps) SourceConnector_defineProperties(Constructor, staticProps); return Constructor; }

function SourceConnector_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }




var SourceConnector = /*#__PURE__*/function () {
  // The drop target may either be attached via ref or connect function
  // The drag preview may either be attached via ref or connect function
  function SourceConnector(backend) {
    var _this = this;

    SourceConnector_classCallCheck(this, SourceConnector);

    SourceConnector_defineProperty(this, "hooks", wrapConnectorHooks({
      dragSource: function dragSource(node, options) {
        _this.clearDragSource();

        _this.dragSourceOptions = options || null;

        if (isRef(node)) {
          _this.dragSourceRef = node;
        } else {
          _this.dragSourceNode = node;
        }

        _this.reconnectDragSource();
      },
      dragPreview: function dragPreview(node, options) {
        _this.clearDragPreview();

        _this.dragPreviewOptions = options || null;

        if (isRef(node)) {
          _this.dragPreviewRef = node;
        } else {
          _this.dragPreviewNode = node;
        }

        _this.reconnectDragPreview();
      }
    }));

    SourceConnector_defineProperty(this, "handlerId", null);

    SourceConnector_defineProperty(this, "dragSourceRef", null);

    SourceConnector_defineProperty(this, "dragSourceNode", void 0);

    SourceConnector_defineProperty(this, "dragSourceOptionsInternal", null);

    SourceConnector_defineProperty(this, "dragSourceUnsubscribe", void 0);

    SourceConnector_defineProperty(this, "dragPreviewRef", null);

    SourceConnector_defineProperty(this, "dragPreviewNode", void 0);

    SourceConnector_defineProperty(this, "dragPreviewOptionsInternal", null);

    SourceConnector_defineProperty(this, "dragPreviewUnsubscribe", void 0);

    SourceConnector_defineProperty(this, "lastConnectedHandlerId", null);

    SourceConnector_defineProperty(this, "lastConnectedDragSource", null);

    SourceConnector_defineProperty(this, "lastConnectedDragSourceOptions", null);

    SourceConnector_defineProperty(this, "lastConnectedDragPreview", null);

    SourceConnector_defineProperty(this, "lastConnectedDragPreviewOptions", null);

    SourceConnector_defineProperty(this, "backend", void 0);

    this.backend = backend;
  }

  SourceConnector_createClass(SourceConnector, [{
    key: "receiveHandlerId",
    value: function receiveHandlerId(newHandlerId) {
      if (this.handlerId === newHandlerId) {
        return;
      }

      this.handlerId = newHandlerId;
      this.reconnect();
    }
  }, {
    key: "connectTarget",
    get: function get() {
      return this.dragSource;
    }
  }, {
    key: "dragSourceOptions",
    get: function get() {
      return this.dragSourceOptionsInternal;
    },
    set: function set(options) {
      this.dragSourceOptionsInternal = options;
    }
  }, {
    key: "dragPreviewOptions",
    get: function get() {
      return this.dragPreviewOptionsInternal;
    },
    set: function set(options) {
      this.dragPreviewOptionsInternal = options;
    }
  }, {
    key: "reconnect",
    value: function reconnect() {
      this.reconnectDragSource();
      this.reconnectDragPreview();
    }
  }, {
    key: "reconnectDragSource",
    value: function reconnectDragSource() {
      var dragSource = this.dragSource; // if nothing has changed then don't resubscribe

      var didChange = this.didHandlerIdChange() || this.didConnectedDragSourceChange() || this.didDragSourceOptionsChange();

      if (didChange) {
        this.disconnectDragSource();
      }

      if (!this.handlerId) {
        return;
      }

      if (!dragSource) {
        this.lastConnectedDragSource = dragSource;
        return;
      }

      if (didChange) {
        this.lastConnectedHandlerId = this.handlerId;
        this.lastConnectedDragSource = dragSource;
        this.lastConnectedDragSourceOptions = this.dragSourceOptions;
        this.dragSourceUnsubscribe = this.backend.connectDragSource(this.handlerId, dragSource, this.dragSourceOptions);
      }
    }
  }, {
    key: "reconnectDragPreview",
    value: function reconnectDragPreview() {
      var dragPreview = this.dragPreview; // if nothing has changed then don't resubscribe

      var didChange = this.didHandlerIdChange() || this.didConnectedDragPreviewChange() || this.didDragPreviewOptionsChange();

      if (didChange) {
        this.disconnectDragPreview();
      }

      if (!this.handlerId) {
        return;
      }

      if (!dragPreview) {
        this.lastConnectedDragPreview = dragPreview;
        return;
      }

      if (didChange) {
        this.lastConnectedHandlerId = this.handlerId;
        this.lastConnectedDragPreview = dragPreview;
        this.lastConnectedDragPreviewOptions = this.dragPreviewOptions;
        this.dragPreviewUnsubscribe = this.backend.connectDragPreview(this.handlerId, dragPreview, this.dragPreviewOptions);
      }
    }
  }, {
    key: "didHandlerIdChange",
    value: function didHandlerIdChange() {
      return this.lastConnectedHandlerId !== this.handlerId;
    }
  }, {
    key: "didConnectedDragSourceChange",
    value: function didConnectedDragSourceChange() {
      return this.lastConnectedDragSource !== this.dragSource;
    }
  }, {
    key: "didConnectedDragPreviewChange",
    value: function didConnectedDragPreviewChange() {
      return this.lastConnectedDragPreview !== this.dragPreview;
    }
  }, {
    key: "didDragSourceOptionsChange",
    value: function didDragSourceOptionsChange() {
      return !shallowEqual(this.lastConnectedDragSourceOptions, this.dragSourceOptions);
    }
  }, {
    key: "didDragPreviewOptionsChange",
    value: function didDragPreviewOptionsChange() {
      return !shallowEqual(this.lastConnectedDragPreviewOptions, this.dragPreviewOptions);
    }
  }, {
    key: "disconnectDragSource",
    value: function disconnectDragSource() {
      if (this.dragSourceUnsubscribe) {
        this.dragSourceUnsubscribe();
        this.dragSourceUnsubscribe = undefined;
      }
    }
  }, {
    key: "disconnectDragPreview",
    value: function disconnectDragPreview() {
      if (this.dragPreviewUnsubscribe) {
        this.dragPreviewUnsubscribe();
        this.dragPreviewUnsubscribe = undefined;
        this.dragPreviewNode = null;
        this.dragPreviewRef = null;
      }
    }
  }, {
    key: "dragSource",
    get: function get() {
      return this.dragSourceNode || this.dragSourceRef && this.dragSourceRef.current;
    }
  }, {
    key: "dragPreview",
    get: function get() {
      return this.dragPreviewNode || this.dragPreviewRef && this.dragPreviewRef.current;
    }
  }, {
    key: "clearDragSource",
    value: function clearDragSource() {
      this.dragSourceNode = null;
      this.dragSourceRef = null;
    }
  }, {
    key: "clearDragPreview",
    value: function clearDragPreview() {
      this.dragPreviewNode = null;
      this.dragPreviewRef = null;
    }
  }]);

  return SourceConnector;
}();
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrag/useDragSourceConnector.js




function useDragSourceConnector(dragSourceOptions, dragPreviewOptions) {
  var manager = useDragDropManager();
  var connector = (0,react.useMemo)(function () {
    return new SourceConnector(manager.getBackend());
  }, [manager]);
  useIsomorphicLayoutEffect(function () {
    connector.dragSourceOptions = dragSourceOptions || null;
    connector.reconnect();
    return function () {
      return connector.disconnectDragSource();
    };
  }, [connector, dragSourceOptions]);
  useIsomorphicLayoutEffect(function () {
    connector.dragPreviewOptions = dragPreviewOptions || null;
    connector.reconnect();
    return function () {
      return connector.disconnectDragPreview();
    };
  }, [connector, dragPreviewOptions]);
  return connector;
}
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/fast-deep-equal@3.1.3/node_modules/fast-deep-equal/index.js
var fast_deep_equal = __webpack_require__(78064);
var fast_deep_equal_default = /*#__PURE__*/__webpack_require__.n(fast_deep_equal);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useCollector.js
function useCollector_slicedToArray(arr, i) { return useCollector_arrayWithHoles(arr) || useCollector_iterableToArrayLimit(arr, i) || useCollector_unsupportedIterableToArray(arr, i) || useCollector_nonIterableRest(); }

function useCollector_nonIterableRest() { throw new TypeError("Invalid attempt to destructure non-iterable instance.\nIn order to be iterable, non-array objects must have a [Symbol.iterator]() method."); }

function useCollector_unsupportedIterableToArray(o, minLen) { if (!o) return; if (typeof o === "string") return useCollector_arrayLikeToArray(o, minLen); var n = Object.prototype.toString.call(o).slice(8, -1); if (n === "Object" && o.constructor) n = o.constructor.name; if (n === "Map" || n === "Set") return Array.from(o); if (n === "Arguments" || /^(?:Ui|I)nt(?:8|16|32)(?:Clamped)?Array$/.test(n)) return useCollector_arrayLikeToArray(o, minLen); }

function useCollector_arrayLikeToArray(arr, len) { if (len == null || len > arr.length) len = arr.length; for (var i = 0, arr2 = new Array(len); i < len; i++) { arr2[i] = arr[i]; } return arr2; }

function useCollector_iterableToArrayLimit(arr, i) { var _i = arr == null ? null : typeof Symbol !== "undefined" && arr[Symbol.iterator] || arr["@@iterator"]; if (_i == null) return; var _arr = []; var _n = true; var _d = false; var _s, _e; try { for (_i = _i.call(arr); !(_n = (_s = _i.next()).done); _n = true) { _arr.push(_s.value); if (i && _arr.length === i) break; } } catch (err) { _d = true; _e = err; } finally { try { if (!_n && _i["return"] != null) _i["return"](); } finally { if (_d) throw _e; } } return _arr; }

function useCollector_arrayWithHoles(arr) { if (Array.isArray(arr)) return arr; }




/**
 *
 * @param monitor The monitor to collect state from
 * @param collect The collecting function
 * @param onUpdate A method to invoke when updates occur
 */

function useCollector(monitor, collect, onUpdate) {
  var _useState = (0,react.useState)(function () {
    return collect(monitor);
  }),
      _useState2 = useCollector_slicedToArray(_useState, 2),
      collected = _useState2[0],
      setCollected = _useState2[1];

  var updateCollected = (0,react.useCallback)(function () {
    var nextValue = collect(monitor); // This needs to be a deep-equality check because some monitor-collected values
    // include XYCoord objects that may be equivalent, but do not have instance equality.

    if (!fast_deep_equal_default()(collected, nextValue)) {
      setCollected(nextValue);

      if (onUpdate) {
        onUpdate();
      }
    }
  }, [collected, monitor, onUpdate]); // update the collected properties after react renders.
  // Note that the "Dustbin Stress Test" fails if this is not
  // done when the component updates

  useIsomorphicLayoutEffect(updateCollected);
  return [collected, updateCollected];
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useMonitorOutput.js
function useMonitorOutput_slicedToArray(arr, i) { return useMonitorOutput_arrayWithHoles(arr) || useMonitorOutput_iterableToArrayLimit(arr, i) || useMonitorOutput_unsupportedIterableToArray(arr, i) || useMonitorOutput_nonIterableRest(); }

function useMonitorOutput_nonIterableRest() { throw new TypeError("Invalid attempt to destructure non-iterable instance.\nIn order to be iterable, non-array objects must have a [Symbol.iterator]() method."); }

function useMonitorOutput_unsupportedIterableToArray(o, minLen) { if (!o) return; if (typeof o === "string") return useMonitorOutput_arrayLikeToArray(o, minLen); var n = Object.prototype.toString.call(o).slice(8, -1); if (n === "Object" && o.constructor) n = o.constructor.name; if (n === "Map" || n === "Set") return Array.from(o); if (n === "Arguments" || /^(?:Ui|I)nt(?:8|16|32)(?:Clamped)?Array$/.test(n)) return useMonitorOutput_arrayLikeToArray(o, minLen); }

function useMonitorOutput_arrayLikeToArray(arr, len) { if (len == null || len > arr.length) len = arr.length; for (var i = 0, arr2 = new Array(len); i < len; i++) { arr2[i] = arr[i]; } return arr2; }

function useMonitorOutput_iterableToArrayLimit(arr, i) { var _i = arr == null ? null : typeof Symbol !== "undefined" && arr[Symbol.iterator] || arr["@@iterator"]; if (_i == null) return; var _arr = []; var _n = true; var _d = false; var _s, _e; try { for (_i = _i.call(arr); !(_n = (_s = _i.next()).done); _n = true) { _arr.push(_s.value); if (i && _arr.length === i) break; } } catch (err) { _d = true; _e = err; } finally { try { if (!_n && _i["return"] != null) _i["return"](); } finally { if (_d) throw _e; } } return _arr; }

function useMonitorOutput_arrayWithHoles(arr) { if (Array.isArray(arr)) return arr; }



function useMonitorOutput(monitor, collect, onCollect) {
  var _useCollector = useCollector(monitor, collect, onCollect),
      _useCollector2 = useMonitorOutput_slicedToArray(_useCollector, 2),
      collected = _useCollector2[0],
      updateCollected = _useCollector2[1];

  useIsomorphicLayoutEffect(function subscribeToMonitorStateChange() {
    var handlerId = monitor.getHandlerId();

    if (handlerId == null) {
      return;
    }

    return monitor.subscribeToStateChange(updateCollected, {
      handlerIds: [handlerId]
    });
  }, [monitor, updateCollected]);
  return collected;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useCollectedProps.js

function useCollectedProps(collector, monitor, connector) {
  return useMonitorOutput(monitor, collector || function () {
    return {};
  }, function () {
    return connector.reconnect();
  });
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrag/connectors.js

function useConnectDragSource(connector) {
  return (0,react.useMemo)(function () {
    return connector.hooks.dragSource();
  }, [connector]);
}
function useConnectDragPreview(connector) {
  return (0,react.useMemo)(function () {
    return connector.hooks.dragPreview();
  }, [connector]);
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrag/useDrag.js







/**
 * useDragSource hook
 * @param sourceSpec The drag source specification (object or function, function preferred)
 * @param deps The memoization deps array to use when evaluating spec changes
 */

function useDrag(specArg, deps) {
  var spec = useOptionalFactory(specArg, deps);
  invariant(!spec.begin, "useDrag::spec.begin was deprecated in v14. Replace spec.begin() with spec.item(). (see more here - https://react-dnd.github.io/react-dnd/docs/api/use-drag)");
  var monitor = useDragSourceMonitor();
  var connector = useDragSourceConnector(spec.options, spec.previewOptions);
  useRegisteredDragSource(spec, monitor, connector);
  return [useCollectedProps(spec.collect, monitor, connector), useConnectDragSource(connector), useConnectDragPreview(connector)];
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/getEmptyImage.js
var emptyImage;
function getEmptyImage() {
  if (!emptyImage) {
    emptyImage = new Image();
    emptyImage.src = 'data:image/gif;base64,R0lGODlhAQABAAAAACH5BAEKAAEALAAAAAABAAEAAAICTAEAOw==';
  }

  return emptyImage;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/dnd/drag-hook.js





function useDragHook(node) {
    const tree = useTreeApi();
    const ids = tree.selectedIds;
    const [_, ref, preview] = useDrag(() => ({
        canDrag: () => node.isDraggable,
        type: "NODE",
        item: () => {
            // This is fired once at the begging of a drag operation
            const dragIds = tree.isSelected(node.id) ? Array.from(ids) : [node.id];
            tree.dispatch(dnd_slice_actions.dragStart(node.id, dragIds));
            return { id: node.id, dragIds };
        },
        end: () => {
            tree.hideCursor();
            tree.dispatch(dnd_slice_actions.dragEnd());
        },
    }), [ids, node]);
    (0,react.useEffect)(() => {
        preview(getEmptyImage());
    }, [preview]);
    return ref;
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrop/useAccept.js


/**
 * Internal utility hook to get an array-version of spec.accept.
 * The main utility here is that we aren't creating a new array on every render if a non-array spec.accept is passed in.
 * @param spec
 */

function useAccept(spec) {
  var accept = spec.accept;
  return (0,react.useMemo)(function () {
    invariant(spec.accept != null, 'accept must be defined');
    return Array.isArray(accept) ? accept : [accept];
  }, [accept]);
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrop/DropTargetImpl.js
function DropTargetImpl_classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function DropTargetImpl_defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function DropTargetImpl_createClass(Constructor, protoProps, staticProps) { if (protoProps) DropTargetImpl_defineProperties(Constructor.prototype, protoProps); if (staticProps) DropTargetImpl_defineProperties(Constructor, staticProps); return Constructor; }

function DropTargetImpl_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }

var DropTargetImpl = /*#__PURE__*/function () {
  function DropTargetImpl(spec, monitor) {
    DropTargetImpl_classCallCheck(this, DropTargetImpl);

    DropTargetImpl_defineProperty(this, "spec", void 0);

    DropTargetImpl_defineProperty(this, "monitor", void 0);

    this.spec = spec;
    this.monitor = monitor;
  }

  DropTargetImpl_createClass(DropTargetImpl, [{
    key: "canDrop",
    value: function canDrop() {
      var spec = this.spec;
      var monitor = this.monitor;
      return spec.canDrop ? spec.canDrop(monitor.getItem(), monitor) : true;
    }
  }, {
    key: "hover",
    value: function hover() {
      var spec = this.spec;
      var monitor = this.monitor;

      if (spec.hover) {
        spec.hover(monitor.getItem(), monitor);
      }
    }
  }, {
    key: "drop",
    value: function drop() {
      var spec = this.spec;
      var monitor = this.monitor;

      if (spec.drop) {
        return spec.drop(monitor.getItem(), monitor);
      }
    }
  }]);

  return DropTargetImpl;
}();
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrop/useDropTarget.js


function useDropTarget(spec, monitor) {
  var dropTarget = (0,react.useMemo)(function () {
    return new DropTargetImpl(spec, monitor);
  }, [monitor]);
  (0,react.useEffect)(function () {
    dropTarget.spec = spec;
  }, [spec]);
  return dropTarget;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrop/useRegisteredDropTarget.js
function useRegisteredDropTarget_slicedToArray(arr, i) { return useRegisteredDropTarget_arrayWithHoles(arr) || useRegisteredDropTarget_iterableToArrayLimit(arr, i) || useRegisteredDropTarget_unsupportedIterableToArray(arr, i) || useRegisteredDropTarget_nonIterableRest(); }

function useRegisteredDropTarget_nonIterableRest() { throw new TypeError("Invalid attempt to destructure non-iterable instance.\nIn order to be iterable, non-array objects must have a [Symbol.iterator]() method."); }

function useRegisteredDropTarget_unsupportedIterableToArray(o, minLen) { if (!o) return; if (typeof o === "string") return useRegisteredDropTarget_arrayLikeToArray(o, minLen); var n = Object.prototype.toString.call(o).slice(8, -1); if (n === "Object" && o.constructor) n = o.constructor.name; if (n === "Map" || n === "Set") return Array.from(o); if (n === "Arguments" || /^(?:Ui|I)nt(?:8|16|32)(?:Clamped)?Array$/.test(n)) return useRegisteredDropTarget_arrayLikeToArray(o, minLen); }

function useRegisteredDropTarget_arrayLikeToArray(arr, len) { if (len == null || len > arr.length) len = arr.length; for (var i = 0, arr2 = new Array(len); i < len; i++) { arr2[i] = arr[i]; } return arr2; }

function useRegisteredDropTarget_iterableToArrayLimit(arr, i) { var _i = arr == null ? null : typeof Symbol !== "undefined" && arr[Symbol.iterator] || arr["@@iterator"]; if (_i == null) return; var _arr = []; var _n = true; var _d = false; var _s, _e; try { for (_i = _i.call(arr); !(_n = (_s = _i.next()).done); _n = true) { _arr.push(_s.value); if (i && _arr.length === i) break; } } catch (err) { _d = true; _e = err; } finally { try { if (!_n && _i["return"] != null) _i["return"](); } finally { if (_d) throw _e; } } return _arr; }

function useRegisteredDropTarget_arrayWithHoles(arr) { if (Array.isArray(arr)) return arr; }






function useRegisteredDropTarget(spec, monitor, connector) {
  var manager = useDragDropManager();
  var dropTarget = useDropTarget(spec, monitor);
  var accept = useAccept(spec);
  useIsomorphicLayoutEffect(function registerDropTarget() {
    var _registerTarget = registerTarget(accept, dropTarget, manager),
        _registerTarget2 = useRegisteredDropTarget_slicedToArray(_registerTarget, 2),
        handlerId = _registerTarget2[0],
        unregister = _registerTarget2[1];

    monitor.receiveHandlerId(handlerId);
    connector.receiveHandlerId(handlerId);
    return unregister;
  }, [manager, monitor, dropTarget, connector, accept.map(function (a) {
    return a.toString();
  }).join('|')]);
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/internals/DropTargetMonitorImpl.js
function DropTargetMonitorImpl_classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function DropTargetMonitorImpl_defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function DropTargetMonitorImpl_createClass(Constructor, protoProps, staticProps) { if (protoProps) DropTargetMonitorImpl_defineProperties(Constructor.prototype, protoProps); if (staticProps) DropTargetMonitorImpl_defineProperties(Constructor, staticProps); return Constructor; }

function DropTargetMonitorImpl_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }


var isCallingCanDrop = false;
var DropTargetMonitorImpl = /*#__PURE__*/function () {
  function DropTargetMonitorImpl(manager) {
    DropTargetMonitorImpl_classCallCheck(this, DropTargetMonitorImpl);

    DropTargetMonitorImpl_defineProperty(this, "internalMonitor", void 0);

    DropTargetMonitorImpl_defineProperty(this, "targetId", null);

    this.internalMonitor = manager.getMonitor();
  }

  DropTargetMonitorImpl_createClass(DropTargetMonitorImpl, [{
    key: "receiveHandlerId",
    value: function receiveHandlerId(targetId) {
      this.targetId = targetId;
    }
  }, {
    key: "getHandlerId",
    value: function getHandlerId() {
      return this.targetId;
    }
  }, {
    key: "subscribeToStateChange",
    value: function subscribeToStateChange(listener, options) {
      return this.internalMonitor.subscribeToStateChange(listener, options);
    }
  }, {
    key: "canDrop",
    value: function canDrop() {
      // Cut out early if the target id has not been set. This should prevent errors
      // where the user has an older version of dnd-core like in
      // https://github.com/react-dnd/react-dnd/issues/1310
      if (!this.targetId) {
        return false;
      }

      invariant(!isCallingCanDrop, 'You may not call monitor.canDrop() inside your canDrop() implementation. ' + 'Read more: http://react-dnd.github.io/react-dnd/docs/api/drop-target-monitor');

      try {
        isCallingCanDrop = true;
        return this.internalMonitor.canDropOnTarget(this.targetId);
      } finally {
        isCallingCanDrop = false;
      }
    }
  }, {
    key: "isOver",
    value: function isOver(options) {
      if (!this.targetId) {
        return false;
      }

      return this.internalMonitor.isOverTarget(this.targetId, options);
    }
  }, {
    key: "getItemType",
    value: function getItemType() {
      return this.internalMonitor.getItemType();
    }
  }, {
    key: "getItem",
    value: function getItem() {
      return this.internalMonitor.getItem();
    }
  }, {
    key: "getDropResult",
    value: function getDropResult() {
      return this.internalMonitor.getDropResult();
    }
  }, {
    key: "didDrop",
    value: function didDrop() {
      return this.internalMonitor.didDrop();
    }
  }, {
    key: "getInitialClientOffset",
    value: function getInitialClientOffset() {
      return this.internalMonitor.getInitialClientOffset();
    }
  }, {
    key: "getInitialSourceClientOffset",
    value: function getInitialSourceClientOffset() {
      return this.internalMonitor.getInitialSourceClientOffset();
    }
  }, {
    key: "getSourceClientOffset",
    value: function getSourceClientOffset() {
      return this.internalMonitor.getSourceClientOffset();
    }
  }, {
    key: "getClientOffset",
    value: function getClientOffset() {
      return this.internalMonitor.getClientOffset();
    }
  }, {
    key: "getDifferenceFromInitialOffset",
    value: function getDifferenceFromInitialOffset() {
      return this.internalMonitor.getDifferenceFromInitialOffset();
    }
  }]);

  return DropTargetMonitorImpl;
}();
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrop/useDropTargetMonitor.js



function useDropTargetMonitor() {
  var manager = useDragDropManager();
  return (0,react.useMemo)(function () {
    return new DropTargetMonitorImpl(manager);
  }, [manager]);
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/internals/TargetConnector.js
function TargetConnector_classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function TargetConnector_defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function TargetConnector_createClass(Constructor, protoProps, staticProps) { if (protoProps) TargetConnector_defineProperties(Constructor.prototype, protoProps); if (staticProps) TargetConnector_defineProperties(Constructor, staticProps); return Constructor; }

function TargetConnector_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }




var TargetConnector = /*#__PURE__*/function () {
  // The drop target may either be attached via ref or connect function
  function TargetConnector(backend) {
    var _this = this;

    TargetConnector_classCallCheck(this, TargetConnector);

    TargetConnector_defineProperty(this, "hooks", wrapConnectorHooks({
      dropTarget: function dropTarget(node, options) {
        _this.clearDropTarget();

        _this.dropTargetOptions = options;

        if (isRef(node)) {
          _this.dropTargetRef = node;
        } else {
          _this.dropTargetNode = node;
        }

        _this.reconnect();
      }
    }));

    TargetConnector_defineProperty(this, "handlerId", null);

    TargetConnector_defineProperty(this, "dropTargetRef", null);

    TargetConnector_defineProperty(this, "dropTargetNode", void 0);

    TargetConnector_defineProperty(this, "dropTargetOptionsInternal", null);

    TargetConnector_defineProperty(this, "unsubscribeDropTarget", void 0);

    TargetConnector_defineProperty(this, "lastConnectedHandlerId", null);

    TargetConnector_defineProperty(this, "lastConnectedDropTarget", null);

    TargetConnector_defineProperty(this, "lastConnectedDropTargetOptions", null);

    TargetConnector_defineProperty(this, "backend", void 0);

    this.backend = backend;
  }

  TargetConnector_createClass(TargetConnector, [{
    key: "connectTarget",
    get: function get() {
      return this.dropTarget;
    }
  }, {
    key: "reconnect",
    value: function reconnect() {
      // if nothing has changed then don't resubscribe
      var didChange = this.didHandlerIdChange() || this.didDropTargetChange() || this.didOptionsChange();

      if (didChange) {
        this.disconnectDropTarget();
      }

      var dropTarget = this.dropTarget;

      if (!this.handlerId) {
        return;
      }

      if (!dropTarget) {
        this.lastConnectedDropTarget = dropTarget;
        return;
      }

      if (didChange) {
        this.lastConnectedHandlerId = this.handlerId;
        this.lastConnectedDropTarget = dropTarget;
        this.lastConnectedDropTargetOptions = this.dropTargetOptions;
        this.unsubscribeDropTarget = this.backend.connectDropTarget(this.handlerId, dropTarget, this.dropTargetOptions);
      }
    }
  }, {
    key: "receiveHandlerId",
    value: function receiveHandlerId(newHandlerId) {
      if (newHandlerId === this.handlerId) {
        return;
      }

      this.handlerId = newHandlerId;
      this.reconnect();
    }
  }, {
    key: "dropTargetOptions",
    get: function get() {
      return this.dropTargetOptionsInternal;
    },
    set: function set(options) {
      this.dropTargetOptionsInternal = options;
    }
  }, {
    key: "didHandlerIdChange",
    value: function didHandlerIdChange() {
      return this.lastConnectedHandlerId !== this.handlerId;
    }
  }, {
    key: "didDropTargetChange",
    value: function didDropTargetChange() {
      return this.lastConnectedDropTarget !== this.dropTarget;
    }
  }, {
    key: "didOptionsChange",
    value: function didOptionsChange() {
      return !shallowEqual(this.lastConnectedDropTargetOptions, this.dropTargetOptions);
    }
  }, {
    key: "disconnectDropTarget",
    value: function disconnectDropTarget() {
      if (this.unsubscribeDropTarget) {
        this.unsubscribeDropTarget();
        this.unsubscribeDropTarget = undefined;
      }
    }
  }, {
    key: "dropTarget",
    get: function get() {
      return this.dropTargetNode || this.dropTargetRef && this.dropTargetRef.current;
    }
  }, {
    key: "clearDropTarget",
    value: function clearDropTarget() {
      this.dropTargetRef = null;
      this.dropTargetNode = null;
    }
  }]);

  return TargetConnector;
}();
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrop/useDropTargetConnector.js




function useDropTargetConnector(options) {
  var manager = useDragDropManager();
  var connector = (0,react.useMemo)(function () {
    return new TargetConnector(manager.getBackend());
  }, [manager]);
  useIsomorphicLayoutEffect(function () {
    connector.dropTargetOptions = options || null;
    connector.reconnect();
    return function () {
      return connector.disconnectDropTarget();
    };
  }, [options]);
  return connector;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrop/connectors.js

function useConnectDropTarget(connector) {
  return (0,react.useMemo)(function () {
    return connector.hooks.dropTarget();
  }, [connector]);
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDrop/useDrop.js






/**
 * useDropTarget Hook
 * @param spec The drop target specification (object or function, function preferred)
 * @param deps The memoization deps array to use when evaluating spec changes
 */

function useDrop(specArg, deps) {
  var spec = useOptionalFactory(specArg, deps);
  var monitor = useDropTargetMonitor();
  var connector = useDropTargetConnector(spec.options);
  useRegisteredDropTarget(spec, monitor, connector);
  return [useCollectedProps(spec.collect, monitor, connector), useConnectDropTarget(connector)];
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/dnd/compute-drop.js

function measureHover(el, offset) {
    const rect = el.getBoundingClientRect();
    const x = offset.x - Math.round(rect.x);
    const y = offset.y - Math.round(rect.y);
    const height = rect.height;
    const inTopHalf = y < height / 2;
    const inBottomHalf = !inTopHalf;
    const pad = height / 4;
    const inMiddle = y > pad && y < height - pad;
    const atTop = !inMiddle && inTopHalf;
    const atBottom = !inMiddle && inBottomHalf;
    return { x, inTopHalf, inBottomHalf, inMiddle, atTop, atBottom };
}
function getNodesAroundCursor(node, prev, next, hover) {
    if (!node) {
        // We're hovering over the empty part of the list, not over an item,
        // Put the cursor below the last item which is "prev"
        return [prev, null];
    }
    if (node.isInternal) {
        if (hover.atTop) {
            return [prev, node];
        }
        else if (hover.inMiddle) {
            return [node, node];
        }
        else {
            return [node, next];
        }
    }
    else {
        if (hover.inTopHalf) {
            return [prev, node];
        }
        else {
            return [node, next];
        }
    }
}
function dropAt(parentId, index) {
    return { parentId: parentId || null, index };
}
function lineCursor(index, level) {
    return {
        type: "line",
        index,
        level,
    };
}
function noCursor() {
    return {
        type: "none",
    };
}
function highlightCursor(id) {
    return {
        type: "highlight",
        id,
    };
}
function walkUpFrom(node, level) {
    var _a;
    let drop = node;
    while (drop.parent && drop.level > level) {
        drop = drop.parent;
    }
    const parentId = ((_a = drop.parent) === null || _a === void 0 ? void 0 : _a.id) || null;
    const index = indexOf(drop) + 1;
    return { parentId, index };
}
/**
 * This is the most complex, tricky function in the whole repo.
 */
function computeDrop(args) {
    var _a;
    const hover = measureHover(args.element, args.offset);
    const indent = args.indent;
    const hoverLevel = Math.round(Math.max(0, hover.x - indent) / indent);
    const { node, nextNode, prevNode } = args;
    const [above, below] = getNodesAroundCursor(node, prevNode, nextNode, hover);
    /* Hovering over the middle of a folder */
    if (node && node.isInternal && hover.inMiddle) {
        return {
            drop: dropAt(node.id, null),
            cursor: highlightCursor(node.id),
        };
    }
    /*
     * Now we only need to care about the node above the cursor
     * -----------                            -------
     */
    /* There is no node above the cursor line */
    if (!above) {
        return {
            drop: dropAt((_a = below === null || below === void 0 ? void 0 : below.parent) === null || _a === void 0 ? void 0 : _a.id, 0),
            cursor: lineCursor(0, 0),
        };
    }
    /* The node above the cursor line is an item */
    if (isItem(above)) {
        const level = bound(hoverLevel, (below === null || below === void 0 ? void 0 : below.level) || 0, above.level);
        return {
            drop: walkUpFrom(above, level),
            cursor: lineCursor(above.rowIndex + 1, level),
        };
    }
    /* The node above the cursor line is a closed folder */
    if (isClosed(above)) {
        const level = bound(hoverLevel, (below === null || below === void 0 ? void 0 : below.level) || 0, above.level);
        return {
            drop: walkUpFrom(above, level),
            cursor: lineCursor(above.rowIndex + 1, level),
        };
    }
    /* The node above the cursor line is an open folder with no children */
    if (isOpenWithEmptyChildren(above)) {
        const level = bound(hoverLevel, 0, above.level + 1);
        if (level > above.level) {
            /* Will be the first child of the empty folder */
            return {
                drop: dropAt(above.id, 0),
                cursor: lineCursor(above.rowIndex + 1, level),
            };
        }
        else {
            /* Will be a sibling or grandsibling of the empty folder */
            return {
                drop: walkUpFrom(above, level),
                cursor: lineCursor(above.rowIndex + 1, level),
            };
        }
    }
    /* The node above the cursor is a an open folder with children */
    return {
        drop: dropAt(above === null || above === void 0 ? void 0 : above.id, 0),
        cursor: lineCursor(above.rowIndex + 1, above.level + 1),
    };
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/dnd/drop-hook.js






function useDropHook(el, node) {
    const tree = useTreeApi();
    const [_, dropRef] = useDrop(() => ({
        accept: "NODE",
        canDrop: () => tree.canDrop(),
        hover: (_item, m) => {
            const offset = m.getClientOffset();
            if (!el.current || !offset)
                return;
            const { cursor, drop } = computeDrop({
                element: el.current,
                offset: offset,
                indent: tree.indent,
                node: node,
                prevNode: node.prev,
                nextNode: node.next,
            });
            if (drop)
                tree.dispatch(dnd_slice_actions.hovering(drop.parentId, drop.index));
            if (m.canDrop()) {
                if (cursor)
                    tree.showCursor(cursor);
            }
            else {
                tree.hideCursor();
            }
        },
        drop: (_, m) => {
            if (!m.canDrop())
                return null;
            let { parentId, index, dragIds } = tree.state.dnd;
            safeRun(tree.props.onMove, {
                dragIds,
                parentId: parentId === ROOT_ID ? null : parentId,
                index: index === null ? 0 : index, // When it's null it was dropped over a folder
                dragNodes: tree.dragNodes,
                parentNode: tree.get(parentId),
            });
            tree.open(parentId);
        },
    }), [node, el.current, tree.props]);
    return dropRef;
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/hooks/use-fresh-node.js


function useFreshNode(index) {
    const tree = useTreeApi();
    const original = tree.at(index);
    if (!original)
        throw new Error(`Could not find node for index: ${index}`);
    return (0,react.useMemo)(() => {
        const fresh = original.clone();
        tree.visibleNodes[index] = fresh; // sneaky
        return fresh;
        // Return a fresh instance if the state values change
    }, [...Object.values(original.state), original]);
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/row-container.js






const RowContainer = react_default().memo(function RowContainer({ index, style, }) {
    /* When will the <Row> will re-render.
     *
     * The row component is memo'd so it will only render
     * when a new instance of the NodeApi class is passed
     * to it.
     *
     * The TreeApi instance is stable. It does not
     * change when the internal state changes.
     *
     * The TreeApi has all the references to the nodes.
     * We need to clone the nodes when their state
     * changes. The node class contains no state itself,
     * It always checks the tree for state. The tree's
     * state will always be up to date.
     */
    useDataUpdates(); // Re-render when tree props or visability changes
    const _ = useNodesContext(); // So that we re-render appropriately
    const tree = useTreeApi(); // Tree already has the fresh state
    const node = useFreshNode(index);
    const el = (0,react.useRef)(null);
    const dragRef = useDragHook(node);
    const dropRef = useDropHook(el, node);
    const innerRef = (0,react.useCallback)((n) => {
        el.current = n;
        dropRef(n);
    }, [dropRef]);
    const indent = tree.indent * node.level;
    const nodeStyle = (0,react.useMemo)(() => ({ paddingLeft: indent }), [indent]);
    const rowStyle = (0,react.useMemo)(() => {
        var _a, _b;
        return (Object.assign(Object.assign({}, style), { top: parseFloat(style.top) +
                ((_b = (_a = tree.props.padding) !== null && _a !== void 0 ? _a : tree.props.paddingTop) !== null && _b !== void 0 ? _b : 0) }));
    }, [style, tree.props.padding, tree.props.paddingTop]);
    const rowAttrs = {
        role: "treeitem",
        "aria-level": node.level + 1,
        "aria-selected": node.isSelected,
        "aria-expanded": node.isOpen,
        style: rowStyle,
        tabIndex: -1,
        className: tree.props.rowClassName,
    };
    (0,react.useEffect)(() => {
        var _a;
        if (!node.isEditing && node.isFocused) {
            (_a = el.current) === null || _a === void 0 ? void 0 : _a.focus({ preventScroll: true });
        }
    }, [node.isEditing, node.isFocused, el.current]);
    const Node = tree.renderNode;
    const Row = tree.renderRow;
    return ((0,react_jsx_runtime.jsx)(Row, { node: node, innerRef: innerRef, attrs: rowAttrs, children: (0,react_jsx_runtime.jsx)(Node, { node: node, tree: tree, style: nodeStyle, dragHandle: dragRef }) }));
});

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/default-container.js







let focusSearchTerm = "";
let timeoutId = null;
/**
 * All these keyboard shortcuts seem like they should be configurable.
 * Each operation should be a given a name and separated from
 * the event handler. Future clean up welcome.
 */
function DefaultContainer() {
    useDataUpdates();
    const tree = useTreeApi();
    return ((0,react_jsx_runtime.jsx)("div", { role: "tree", style: {
            height: tree.height,
            width: tree.width,
            minHeight: 0,
            minWidth: 0,
        }, onContextMenu: tree.props.onContextMenu, onClick: tree.props.onClick, tabIndex: 0, onFocus: (e) => {
            if (!e.currentTarget.contains(e.relatedTarget)) {
                tree.onFocus();
            }
        }, onBlur: (e) => {
            if (!e.currentTarget.contains(e.relatedTarget)) {
                tree.onBlur();
            }
        }, onKeyDown: (e) => {
            var _a;
            if (tree.isEditing) {
                return;
            }
            if (e.key === "Backspace") {
                if (!tree.props.onDelete)
                    return;
                const ids = Array.from(tree.selectedIds);
                if (ids.length > 1) {
                    let nextFocus = tree.mostRecentNode;
                    while (nextFocus && nextFocus.isSelected) {
                        nextFocus = nextFocus.nextSibling;
                    }
                    if (!nextFocus)
                        nextFocus = tree.lastNode;
                    tree.focus(nextFocus, { scroll: false });
                    tree.delete(Array.from(ids));
                }
                else {
                    const node = tree.focusedNode;
                    if (node) {
                        const sib = node.nextSibling;
                        const parent = node.parent;
                        tree.focus(sib || parent, { scroll: false });
                        tree.delete(node);
                    }
                }
                return;
            }
            if (e.key === "Tab" && !e.shiftKey) {
                e.preventDefault();
                focusNextElement(e.currentTarget);
                return;
            }
            if (e.key === "Tab" && e.shiftKey) {
                e.preventDefault();
                focusPrevElement(e.currentTarget);
                return;
            }
            if (e.key === "ArrowDown") {
                e.preventDefault();
                const next = tree.nextNode;
                if (e.metaKey) {
                    tree.select(tree.focusedNode);
                    tree.activate(tree.focusedNode);
                    return;
                }
                else if (!e.shiftKey || tree.props.disableMultiSelection) {
                    tree.focus(next);
                    return;
                }
                else {
                    if (!next)
                        return;
                    const current = tree.focusedNode;
                    if (!current) {
                        tree.focus(tree.firstNode);
                    }
                    else if (current.isSelected) {
                        tree.selectContiguous(next);
                    }
                    else {
                        tree.selectMulti(next);
                    }
                    return;
                }
            }
            if (e.key === "ArrowUp") {
                e.preventDefault();
                const prev = tree.prevNode;
                if (!e.shiftKey || tree.props.disableMultiSelection) {
                    tree.focus(prev);
                    return;
                }
                else {
                    if (!prev)
                        return;
                    const current = tree.focusedNode;
                    if (!current) {
                        tree.focus(tree.lastNode); // ?
                    }
                    else if (current.isSelected) {
                        tree.selectContiguous(prev);
                    }
                    else {
                        tree.selectMulti(prev);
                    }
                    return;
                }
            }
            if (e.key === "ArrowRight") {
                const node = tree.focusedNode;
                if (!node)
                    return;
                if (node.isInternal && node.isOpen) {
                    tree.focus(tree.nextNode);
                }
                else if (node.isInternal)
                    tree.open(node.id);
                return;
            }
            if (e.key === "ArrowLeft") {
                const node = tree.focusedNode;
                if (!node || node.isRoot)
                    return;
                if (node.isInternal && node.isOpen)
                    tree.close(node.id);
                else if (!((_a = node.parent) === null || _a === void 0 ? void 0 : _a.isRoot)) {
                    tree.focus(node.parent);
                }
                return;
            }
            if (e.key === "a" && e.metaKey && !tree.props.disableMultiSelection) {
                e.preventDefault();
                tree.selectAll();
                return;
            }
            if (e.key === "a" && !e.metaKey && tree.props.onCreate) {
                tree.createLeaf();
                return;
            }
            if (e.key === "A" && !e.metaKey) {
                if (!tree.props.onCreate)
                    return;
                tree.createInternal();
                return;
            }
            if (e.key === "Home") {
                // add shift keys
                e.preventDefault();
                tree.focus(tree.firstNode);
                return;
            }
            if (e.key === "End") {
                // add shift keys
                e.preventDefault();
                tree.focus(tree.lastNode);
                return;
            }
            if (e.key === "Enter") {
                const node = tree.focusedNode;
                if (!node)
                    return;
                if (!node.isEditable || !tree.props.onRename)
                    return;
                setTimeout(() => {
                    if (node)
                        tree.edit(node);
                });
                return;
            }
            if (e.key === " ") {
                e.preventDefault();
                const node = tree.focusedNode;
                if (!node)
                    return;
                if (node.isLeaf) {
                    node.select();
                    node.activate();
                }
                else {
                    node.toggle();
                }
                return;
            }
            if (e.key === "*") {
                const node = tree.focusedNode;
                if (!node)
                    return;
                tree.openSiblings(node);
                return;
            }
            if (e.key === "PageUp") {
                e.preventDefault();
                tree.pageUp();
                return;
            }
            if (e.key === "PageDown") {
                e.preventDefault();
                tree.pageDown();
            }
            // If they type a sequence of characters
            // collect them. Reset them after a timeout.
            // Use it to search the tree for a node, then focus it.
            // Clean this up a bit later
            clearTimeout(timeoutId);
            focusSearchTerm += e.key;
            timeoutId = setTimeout(() => {
                focusSearchTerm = "";
            }, 600);
            const node = tree.visibleNodes.find((n) => {
                // @ts-ignore
                const name = n.data.name;
                if (typeof name === "string") {
                    return name.toLowerCase().startsWith(focusSearchTerm);
                }
                else
                    return false;
            });
            if (node)
                tree.focus(node.id);
        }, children: (0,react_jsx_runtime.jsx)(FixedSizeList, { className: tree.props.className, outerRef: tree.listEl, itemCount: tree.visibleNodes.length, height: tree.height, width: tree.width, itemSize: tree.rowHeight, overscanCount: tree.overscanCount, itemKey: (index) => { var _a; return ((_a = tree.visibleNodes[index]) === null || _a === void 0 ? void 0 : _a.id) || index; }, outerElementType: ListOuterElement, innerElementType: ListInnerElement, onScroll: tree.props.onScroll, onItemsRendered: tree.onItemsRendered.bind(tree), ref: tree.list, children: RowContainer }) }));
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/data/create-list.js
function createList(tree) {
    if (tree.isFiltered) {
        return flattenAndFilterTree(tree.root, tree.isMatch.bind(tree));
    }
    else {
        return flattenTree(tree.root);
    }
}
function flattenTree(root) {
    const list = [];
    function collect(node) {
        var _a;
        if (node.level >= 0) {
            list.push(node);
        }
        if (node.isOpen) {
            (_a = node.children) === null || _a === void 0 ? void 0 : _a.forEach(collect);
        }
    }
    collect(root);
    list.forEach(assignRowIndex);
    return list;
}
function flattenAndFilterTree(root, isMatch) {
    const matches = {};
    const list = [];
    function markMatch(node) {
        const yes = !node.isRoot && isMatch(node);
        if (yes) {
            matches[node.id] = true;
            let parent = node.parent;
            while (parent) {
                matches[parent.id] = true;
                parent = parent.parent;
            }
        }
        if (node.children) {
            for (let child of node.children)
                markMatch(child);
        }
    }
    function collect(node) {
        var _a;
        if (node.level >= 0 && matches[node.id]) {
            list.push(node);
        }
        if (node.isOpen) {
            (_a = node.children) === null || _a === void 0 ? void 0 : _a.forEach(collect);
        }
    }
    markMatch(root);
    collect(root);
    list.forEach(assignRowIndex);
    return list;
}
function assignRowIndex(node, index) {
    node.rowIndex = index;
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/data/create-index.js
const createIndex = (nodes) => {
    return nodes.reduce((map, node, index) => {
        map[node.id] = index;
        return map;
    }, {});
};

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/interfaces/tree-api.js
var __awaiter = (undefined && undefined.__awaiter) || function (thisArg, _arguments, P, generator) {
    function adopt(value) { return value instanceof P ? value : new P(function (resolve) { resolve(value); }); }
    return new (P || (P = Promise))(function (resolve, reject) {
        function fulfilled(value) { try { step(generator.next(value)); } catch (e) { reject(e); } }
        function rejected(value) { try { step(generator["throw"](value)); } catch (e) { reject(e); } }
        function step(result) { result.done ? resolve(result.value) : adopt(result.value).then(fulfilled, rejected); }
        step((generator = generator.apply(thisArg, _arguments || [])).next());
    });
};














const { /* safeRun */ "FS": tree_api_safeRun, /* identify */ "ls": tree_api_identify, /* identifyNull */ "yQ": tree_api_identifyNull } = utils_namespaceObject;
class TreeApi {
    constructor(store, props, list, listEl) {
        this.store = store;
        this.props = props;
        this.list = list;
        this.listEl = listEl;
        this.visibleStartIndex = 0;
        this.visibleStopIndex = 0;
        /* Changes here must also be made in update() */
        this.root = createRoot(this);
        this.visibleNodes = createList(this);
        this.idToIndex = createIndex(this.visibleNodes);
    }
    /* Changes here must also be made in constructor() */
    update(props) {
        this.props = props;
        this.root = createRoot(this);
        this.visibleNodes = createList(this);
        this.idToIndex = createIndex(this.visibleNodes);
    }
    /* Store helpers */
    dispatch(action) {
        return this.store.dispatch(action);
    }
    get state() {
        return this.store.getState();
    }
    get openState() {
        return this.state.nodes.open.unfiltered;
    }
    /* Tree Props */
    get width() {
        var _a;
        return (_a = this.props.width) !== null && _a !== void 0 ? _a : 300;
    }
    get height() {
        var _a;
        return (_a = this.props.height) !== null && _a !== void 0 ? _a : 500;
    }
    get indent() {
        var _a;
        return (_a = this.props.indent) !== null && _a !== void 0 ? _a : 24;
    }
    get rowHeight() {
        var _a;
        return (_a = this.props.rowHeight) !== null && _a !== void 0 ? _a : 24;
    }
    get overscanCount() {
        var _a;
        return (_a = this.props.overscanCount) !== null && _a !== void 0 ? _a : 1;
    }
    get searchTerm() {
        return (this.props.searchTerm || "").trim();
    }
    get matchFn() {
        var _a;
        const match = (_a = this.props.searchMatch) !== null && _a !== void 0 ? _a : ((node, term) => {
            const string = JSON.stringify(Object.values(node.data));
            return string.toLocaleLowerCase().includes(term.toLocaleLowerCase());
        });
        return (node) => match(node, this.searchTerm);
    }
    accessChildren(data) {
        var _a;
        const get = this.props.childrenAccessor || "children";
        return (_a = access(data, get)) !== null && _a !== void 0 ? _a : null;
    }
    accessId(data) {
        const get = this.props.idAccessor || "id";
        const id = access(data, get);
        if (!id)
            throw new Error("Data must contain an 'id' property or props.idAccessor must return a string");
        return id;
    }
    /* Node Access */
    get firstNode() {
        var _a;
        return (_a = this.visibleNodes[0]) !== null && _a !== void 0 ? _a : null;
    }
    get lastNode() {
        var _a;
        return (_a = this.visibleNodes[this.visibleNodes.length - 1]) !== null && _a !== void 0 ? _a : null;
    }
    get focusedNode() {
        var _a;
        return (_a = this.get(this.state.nodes.focus.id)) !== null && _a !== void 0 ? _a : null;
    }
    get mostRecentNode() {
        var _a;
        return (_a = this.get(this.state.nodes.selection.mostRecent)) !== null && _a !== void 0 ? _a : null;
    }
    get nextNode() {
        const index = this.indexOf(this.focusedNode);
        if (index === null)
            return null;
        else
            return this.at(index + 1);
    }
    get prevNode() {
        const index = this.indexOf(this.focusedNode);
        if (index === null)
            return null;
        else
            return this.at(index - 1);
    }
    get(id) {
        if (!id)
            return null;
        if (id in this.idToIndex)
            return this.visibleNodes[this.idToIndex[id]] || null;
        else
            return null;
    }
    at(index) {
        return this.visibleNodes[index] || null;
    }
    nodesBetween(startId, endId) {
        var _a;
        if (startId === null || endId === null)
            return [];
        const index1 = (_a = this.indexOf(startId)) !== null && _a !== void 0 ? _a : 0;
        const index2 = this.indexOf(endId);
        if (index2 === null)
            return [];
        const start = Math.min(index1, index2);
        const end = Math.max(index1, index2);
        return this.visibleNodes.slice(start, end + 1);
    }
    indexOf(id) {
        const key = identifyNull(id);
        if (!key)
            return null;
        return this.idToIndex[key];
    }
    /* Data Operations */
    get editingId() {
        return this.state.nodes.edit.id;
    }
    createInternal() {
        return this.create({ type: "internal" });
    }
    createLeaf() {
        return this.create({ type: "leaf" });
    }
    create() {
        return __awaiter(this, arguments, void 0, function* (opts = {}) {
            var _a, _b;
            const parentId = opts.parentId === undefined
                ? getInsertParentId(this)
                : opts.parentId;
            const index = (_a = opts.index) !== null && _a !== void 0 ? _a : getInsertIndex(this);
            const type = (_b = opts.type) !== null && _b !== void 0 ? _b : "leaf";
            const data = yield tree_api_safeRun(this.props.onCreate, {
                type,
                parentId,
                index,
                parentNode: this.get(parentId),
            });
            if (data) {
                this.focus(data);
                setTimeout(() => {
                    this.edit(data).then(() => {
                        this.select(data);
                        this.activate(data);
                    });
                });
            }
        });
    }
    delete(node) {
        return __awaiter(this, void 0, void 0, function* () {
            if (!node)
                return;
            const idents = Array.isArray(node) ? node : [node];
            const ids = idents.map(tree_api_identify);
            const nodes = ids.map((id) => this.get(id)).filter((n) => !!n);
            yield tree_api_safeRun(this.props.onDelete, { nodes, ids });
        });
    }
    edit(node) {
        const id = tree_api_identify(node);
        this.resolveEdit({ cancelled: true });
        this.scrollTo(id);
        this.dispatch(edit(id));
        return new Promise((resolve) => {
            TreeApi.editPromise = resolve;
        });
    }
    submit(identity, value) {
        return __awaiter(this, void 0, void 0, function* () {
            if (!identity)
                return;
            const id = tree_api_identify(identity);
            yield tree_api_safeRun(this.props.onRename, {
                id,
                name: value,
                node: this.get(id),
            });
            this.dispatch(edit(null));
            this.resolveEdit({ cancelled: false, value });
            setTimeout(() => this.onFocus()); // Return focus to element;
        });
    }
    reset() {
        this.dispatch(edit(null));
        this.resolveEdit({ cancelled: true });
        setTimeout(() => this.onFocus()); // Return focus to element;
    }
    activate(id) {
        const node = this.get(tree_api_identifyNull(id));
        if (!node)
            return;
        tree_api_safeRun(this.props.onActivate, node);
    }
    resolveEdit(value) {
        const resolve = TreeApi.editPromise;
        if (resolve)
            resolve(value);
        TreeApi.editPromise = null;
    }
    /* Focus and Selection */
    get selectedIds() {
        return this.state.nodes.selection.ids;
    }
    get selectedNodes() {
        let nodes = [];
        for (let id of Array.from(this.selectedIds)) {
            const node = this.get(id);
            if (node)
                nodes.push(node);
        }
        return nodes;
    }
    focus(node, opts = {}) {
        if (!node)
            return;
        /* Focus is responsible for scrolling, while selection is
         * responsible for focus. If selectionFollowsFocus, then
         * just select it. */
        if (this.props.selectionFollowsFocus) {
            this.select(node);
        }
        else {
            this.dispatch(focus_slice_focus(tree_api_identify(node)));
            if (opts.scroll !== false)
                this.scrollTo(node);
            if (this.focusedNode)
                tree_api_safeRun(this.props.onFocus, this.focusedNode);
        }
    }
    pageUp() {
        var _a, _b;
        const start = this.visibleStartIndex;
        const stop = this.visibleStopIndex;
        const page = stop - start;
        let index = (_b = (_a = this.focusedNode) === null || _a === void 0 ? void 0 : _a.rowIndex) !== null && _b !== void 0 ? _b : 0;
        if (index > start) {
            index = start;
        }
        else {
            index = Math.max(start - page, 0);
        }
        this.focus(this.at(index));
    }
    pageDown() {
        var _a, _b;
        const start = this.visibleStartIndex;
        const stop = this.visibleStopIndex;
        const page = stop - start;
        let index = (_b = (_a = this.focusedNode) === null || _a === void 0 ? void 0 : _a.rowIndex) !== null && _b !== void 0 ? _b : 0;
        if (index < stop) {
            index = stop;
        }
        else {
            index = Math.min(index + page, this.visibleNodes.length - 1);
        }
        this.focus(this.at(index));
    }
    select(node, opts = {}) {
        if (!node)
            return;
        const changeFocus = opts.focus !== false;
        const id = tree_api_identify(node);
        if (changeFocus)
            this.dispatch(focus_slice_focus(id));
        this.dispatch(selection_slice_actions.only(id));
        this.dispatch(selection_slice_actions.anchor(id));
        this.dispatch(selection_slice_actions.mostRecent(id));
        this.scrollTo(id, opts.align);
        if (this.focusedNode && changeFocus) {
            tree_api_safeRun(this.props.onFocus, this.focusedNode);
        }
        tree_api_safeRun(this.props.onSelect, this.selectedNodes);
    }
    deselect(node) {
        if (!node)
            return;
        const id = tree_api_identify(node);
        this.dispatch(selection_slice_actions.remove(id));
        tree_api_safeRun(this.props.onSelect, this.selectedNodes);
    }
    selectMulti(identity) {
        const node = this.get(tree_api_identifyNull(identity));
        if (!node)
            return;
        this.dispatch(focus_slice_focus(node.id));
        this.dispatch(selection_slice_actions.add(node.id));
        this.dispatch(selection_slice_actions.anchor(node.id));
        this.dispatch(selection_slice_actions.mostRecent(node.id));
        this.scrollTo(node);
        if (this.focusedNode)
            tree_api_safeRun(this.props.onFocus, this.focusedNode);
        tree_api_safeRun(this.props.onSelect, this.selectedNodes);
    }
    selectContiguous(identity) {
        if (!identity)
            return;
        const id = tree_api_identify(identity);
        const { anchor, mostRecent } = this.state.nodes.selection;
        this.dispatch(focus_slice_focus(id));
        this.dispatch(selection_slice_actions.remove(this.nodesBetween(anchor, mostRecent)));
        this.dispatch(selection_slice_actions.add(this.nodesBetween(anchor, tree_api_identifyNull(id))));
        this.dispatch(selection_slice_actions.mostRecent(id));
        this.scrollTo(id);
        if (this.focusedNode)
            tree_api_safeRun(this.props.onFocus, this.focusedNode);
        tree_api_safeRun(this.props.onSelect, this.selectedNodes);
    }
    deselectAll() {
        this.setSelection({ ids: [], anchor: null, mostRecent: null });
        tree_api_safeRun(this.props.onSelect, this.selectedNodes);
    }
    selectAll() {
        var _a;
        this.setSelection({
            ids: Object.keys(this.idToIndex),
            anchor: this.firstNode,
            mostRecent: this.lastNode,
        });
        this.dispatch(focus_slice_focus((_a = this.lastNode) === null || _a === void 0 ? void 0 : _a.id));
        if (this.focusedNode)
            tree_api_safeRun(this.props.onFocus, this.focusedNode);
        tree_api_safeRun(this.props.onSelect, this.selectedNodes);
    }
    setSelection(args) {
        var _a;
        const ids = new Set((_a = args.ids) === null || _a === void 0 ? void 0 : _a.map(tree_api_identify));
        const anchor = tree_api_identifyNull(args.anchor);
        const mostRecent = tree_api_identifyNull(args.mostRecent);
        this.dispatch(selection_slice_actions.set({ ids, anchor, mostRecent }));
        tree_api_safeRun(this.props.onSelect, this.selectedNodes);
    }
    /* Drag and Drop */
    get cursorParentId() {
        const { cursor } = this.state.dnd;
        switch (cursor.type) {
            case "highlight":
                return cursor.id;
            default:
                return null;
        }
    }
    get cursorOverFolder() {
        return this.state.dnd.cursor.type === "highlight";
    }
    get dragNodes() {
        return this.state.dnd.dragIds
            .map((id) => this.get(id))
            .filter((n) => !!n);
    }
    get dragNode() {
        return this.get(this.state.nodes.drag.id);
    }
    get dragDestinationParent() {
        return this.get(this.state.nodes.drag.destinationParentId);
    }
    get dragDestinationIndex() {
        return this.state.nodes.drag.destinationIndex;
    }
    canDrop() {
        var _a;
        if (this.isFiltered)
            return false;
        const parentNode = (_a = this.get(this.state.dnd.parentId)) !== null && _a !== void 0 ? _a : this.root;
        const dragNodes = this.dragNodes;
        const isDisabled = this.props.disableDrop;
        for (const drag of dragNodes) {
            if (!drag)
                return false;
            if (!parentNode)
                return false;
            if (drag.isInternal && isDescendant(parentNode, drag))
                return false;
        }
        // Allow the user to insert their own logic
        if (typeof isDisabled == "function") {
            return !isDisabled({
                parentNode,
                dragNodes: this.dragNodes,
                index: this.state.dnd.index || 0,
            });
        }
        else if (typeof isDisabled == "string") {
            // @ts-ignore
            return !parentNode.data[isDisabled];
        }
        else if (typeof isDisabled === "boolean") {
            return !isDisabled;
        }
        else {
            return true;
        }
    }
    hideCursor() {
        this.dispatch(dnd_slice_actions.cursor({ type: "none" }));
    }
    showCursor(cursor) {
        this.dispatch(dnd_slice_actions.cursor(cursor));
    }
    /* Visibility */
    open(identity) {
        const id = tree_api_identifyNull(identity);
        if (!id)
            return;
        if (this.isOpen(id))
            return;
        this.dispatch(actions.open(id, this.isFiltered));
        tree_api_safeRun(this.props.onToggle, id);
    }
    close(identity) {
        const id = tree_api_identifyNull(identity);
        if (!id)
            return;
        if (!this.isOpen(id))
            return;
        this.dispatch(actions.close(id, this.isFiltered));
        tree_api_safeRun(this.props.onToggle, id);
    }
    toggle(identity) {
        const id = tree_api_identifyNull(identity);
        if (!id)
            return;
        return this.isOpen(id) ? this.close(id) : this.open(id);
    }
    openParents(identity) {
        const id = tree_api_identifyNull(identity);
        if (!id)
            return;
        const node = dfs(this.root, id);
        let parent = node === null || node === void 0 ? void 0 : node.parent;
        while (parent) {
            this.open(parent.id);
            parent = parent.parent;
        }
    }
    openSiblings(node) {
        const parent = node.parent;
        if (!parent) {
            this.toggle(node.id);
        }
        else if (parent.children) {
            const isOpen = node.isOpen;
            for (let sibling of parent.children) {
                if (sibling.isInternal) {
                    isOpen ? this.close(sibling.id) : this.open(sibling.id);
                }
            }
            this.scrollTo(this.focusedNode);
        }
    }
    openAll() {
        walk(this.root, (node) => {
            if (node.isInternal)
                node.open();
        });
    }
    closeAll() {
        walk(this.root, (node) => {
            if (node.isInternal)
                node.close();
        });
    }
    /* Scrolling */
    scrollTo(identity, align = "smart") {
        if (!identity)
            return;
        const id = tree_api_identify(identity);
        this.openParents(id);
        return waitFor(() => id in this.idToIndex)
            .then(() => {
            var _a;
            const index = this.idToIndex[id];
            if (index === undefined)
                return;
            (_a = this.list.current) === null || _a === void 0 ? void 0 : _a.scrollToItem(index, align);
        })
            .catch(() => {
            // Id: ${id} never appeared in the list.
        });
    }
    /* State Checks */
    get isEditing() {
        return this.state.nodes.edit.id !== null;
    }
    get isFiltered() {
        var _a;
        return !!((_a = this.props.searchTerm) === null || _a === void 0 ? void 0 : _a.trim());
    }
    get hasFocus() {
        return this.state.nodes.focus.treeFocused;
    }
    get hasNoSelection() {
        return this.state.nodes.selection.ids.size === 0;
    }
    get hasOneSelection() {
        return this.state.nodes.selection.ids.size === 1;
    }
    get hasMultipleSelections() {
        return this.state.nodes.selection.ids.size > 1;
    }
    isSelected(id) {
        if (!id)
            return false;
        return this.state.nodes.selection.ids.has(id);
    }
    isOpen(id) {
        var _a, _b, _c;
        if (!id)
            return false;
        if (id === ROOT_ID)
            return true;
        const def = (_a = this.props.openByDefault) !== null && _a !== void 0 ? _a : true;
        if (this.isFiltered) {
            return (_b = this.state.nodes.open.filtered[id]) !== null && _b !== void 0 ? _b : true; // Filtered folders are always opened by default
        }
        else {
            return (_c = this.state.nodes.open.unfiltered[id]) !== null && _c !== void 0 ? _c : def;
        }
    }
    isEditable(data) {
        const check = this.props.disableEdit || (() => false);
        return !access(data, check);
    }
    isDraggable(data) {
        const check = this.props.disableDrag || (() => false);
        return !access(data, check);
    }
    isDragging(node) {
        const id = tree_api_identifyNull(node);
        if (!id)
            return false;
        return this.state.nodes.drag.id === id;
    }
    isFocused(id) {
        return this.hasFocus && this.state.nodes.focus.id === id;
    }
    isMatch(node) {
        return this.matchFn(node);
    }
    willReceiveDrop(node) {
        const id = tree_api_identifyNull(node);
        if (!id)
            return false;
        const { destinationParentId, destinationIndex } = this.state.nodes.drag;
        return id === destinationParentId && destinationIndex === null;
    }
    /* Tree Event Handlers */
    onFocus() {
        const node = this.focusedNode || this.firstNode;
        if (node)
            this.dispatch(focus_slice_focus(node.id));
    }
    onBlur() {
        this.dispatch(treeBlur());
    }
    onItemsRendered(args) {
        this.visibleStartIndex = args.visibleStartIndex;
        this.visibleStopIndex = args.visibleStopIndex;
    }
    /* Get Renderers */
    get renderContainer() {
        return this.props.renderContainer || DefaultContainer;
    }
    get renderRow() {
        return this.props.renderRow || DefaultRow;
    }
    get renderNode() {
        return this.props.children || DefaultNode;
    }
    get renderDragPreview() {
        return this.props.renderDragPreview || DefaultDragPreview;
    }
    get renderCursor() {
        return this.props.renderCursor || DefaultCursor;
    }
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/redux@5.0.1/node_modules/redux/dist/redux.mjs
var redux = __webpack_require__(61737);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/state/drag-slice.js

/* Reducer */
function drag_slice_reducer(state = initialState().nodes.drag, action) {
    switch (action.type) {
        case "DND_DRAG_START":
            return Object.assign(Object.assign({}, state), { id: action.id, selectedIds: action.dragIds });
        case "DND_DRAG_END":
            return Object.assign(Object.assign({}, state), { id: null, destinationParentId: null, destinationIndex: null, selectedIds: [] });
        case "DND_HOVERING":
            if (action.parentId !== state.destinationParentId ||
                action.index != state.destinationIndex) {
                return Object.assign(Object.assign({}, state), { destinationParentId: action.parentId, destinationIndex: action.index });
            }
            else {
                return state;
            }
        default:
            return state;
    }
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/state/root-reducer.js







const rootReducer = (0,redux/* combineReducers */.HY)({
    nodes: (0,redux/* combineReducers */.HY)({
        focus: focus_slice_reducer,
        edit: reducer,
        open: open_slice_reducer,
        selection: selection_slice_reducer,
        drag: drag_slice_reducer,
    }),
    dnd: dnd_slice_reducer,
});

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/utils/js_utils.js
// cheap lodash replacements
function memoize(fn) {
  var result = null;

  var memoized = function memoized() {
    if (result == null) {
      result = fn();
    }

    return result;
  };

  return memoized;
}
/**
 * drop-in replacement for _.without
 */

function without(items, item) {
  return items.filter(function (i) {
    return i !== item;
  });
}
function union(itemsA, itemsB) {
  var set = new Set();

  var insertItem = function insertItem(item) {
    return set.add(item);
  };

  itemsA.forEach(insertItem);
  itemsB.forEach(insertItem);
  var result = [];
  set.forEach(function (key) {
    return result.push(key);
  });
  return result;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/EnterLeaveCounter.js
function EnterLeaveCounter_classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function EnterLeaveCounter_defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function EnterLeaveCounter_createClass(Constructor, protoProps, staticProps) { if (protoProps) EnterLeaveCounter_defineProperties(Constructor.prototype, protoProps); if (staticProps) EnterLeaveCounter_defineProperties(Constructor, staticProps); return Constructor; }

function EnterLeaveCounter_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }


var EnterLeaveCounter = /*#__PURE__*/function () {
  function EnterLeaveCounter(isNodeInDocument) {
    EnterLeaveCounter_classCallCheck(this, EnterLeaveCounter);

    EnterLeaveCounter_defineProperty(this, "entered", []);

    EnterLeaveCounter_defineProperty(this, "isNodeInDocument", void 0);

    this.isNodeInDocument = isNodeInDocument;
  }

  EnterLeaveCounter_createClass(EnterLeaveCounter, [{
    key: "enter",
    value: function enter(enteringNode) {
      var _this = this;

      var previousLength = this.entered.length;

      var isNodeEntered = function isNodeEntered(node) {
        return _this.isNodeInDocument(node) && (!node.contains || node.contains(enteringNode));
      };

      this.entered = union(this.entered.filter(isNodeEntered), [enteringNode]);
      return previousLength === 0 && this.entered.length > 0;
    }
  }, {
    key: "leave",
    value: function leave(leavingNode) {
      var previousLength = this.entered.length;
      this.entered = without(this.entered.filter(this.isNodeInDocument), leavingNode);
      return previousLength > 0 && this.entered.length === 0;
    }
  }, {
    key: "reset",
    value: function reset() {
      this.entered = [];
    }
  }]);

  return EnterLeaveCounter;
}();
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/BrowserDetector.js

var isFirefox = memoize(function () {
  return /firefox/i.test(navigator.userAgent);
});
var isSafari = memoize(function () {
  return Boolean(window.safari);
});
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/MonotonicInterpolant.js
function MonotonicInterpolant_classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function MonotonicInterpolant_defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function MonotonicInterpolant_createClass(Constructor, protoProps, staticProps) { if (protoProps) MonotonicInterpolant_defineProperties(Constructor.prototype, protoProps); if (staticProps) MonotonicInterpolant_defineProperties(Constructor, staticProps); return Constructor; }

function MonotonicInterpolant_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }

var MonotonicInterpolant = /*#__PURE__*/function () {
  function MonotonicInterpolant(xs, ys) {
    MonotonicInterpolant_classCallCheck(this, MonotonicInterpolant);

    MonotonicInterpolant_defineProperty(this, "xs", void 0);

    MonotonicInterpolant_defineProperty(this, "ys", void 0);

    MonotonicInterpolant_defineProperty(this, "c1s", void 0);

    MonotonicInterpolant_defineProperty(this, "c2s", void 0);

    MonotonicInterpolant_defineProperty(this, "c3s", void 0);

    var length = xs.length; // Rearrange xs and ys so that xs is sorted

    var indexes = [];

    for (var i = 0; i < length; i++) {
      indexes.push(i);
    }

    indexes.sort(function (a, b) {
      return xs[a] < xs[b] ? -1 : 1;
    }); // Get consecutive differences and slopes

    var dys = [];
    var dxs = [];
    var ms = [];
    var dx;
    var dy;

    for (var _i = 0; _i < length - 1; _i++) {
      dx = xs[_i + 1] - xs[_i];
      dy = ys[_i + 1] - ys[_i];
      dxs.push(dx);
      dys.push(dy);
      ms.push(dy / dx);
    } // Get degree-1 coefficients


    var c1s = [ms[0]];

    for (var _i2 = 0; _i2 < dxs.length - 1; _i2++) {
      var m2 = ms[_i2];
      var mNext = ms[_i2 + 1];

      if (m2 * mNext <= 0) {
        c1s.push(0);
      } else {
        dx = dxs[_i2];
        var dxNext = dxs[_i2 + 1];
        var common = dx + dxNext;
        c1s.push(3 * common / ((common + dxNext) / m2 + (common + dx) / mNext));
      }
    }

    c1s.push(ms[ms.length - 1]); // Get degree-2 and degree-3 coefficients

    var c2s = [];
    var c3s = [];
    var m;

    for (var _i3 = 0; _i3 < c1s.length - 1; _i3++) {
      m = ms[_i3];
      var c1 = c1s[_i3];
      var invDx = 1 / dxs[_i3];

      var _common = c1 + c1s[_i3 + 1] - m - m;

      c2s.push((m - c1 - _common) * invDx);
      c3s.push(_common * invDx * invDx);
    }

    this.xs = xs;
    this.ys = ys;
    this.c1s = c1s;
    this.c2s = c2s;
    this.c3s = c3s;
  }

  MonotonicInterpolant_createClass(MonotonicInterpolant, [{
    key: "interpolate",
    value: function interpolate(x) {
      var xs = this.xs,
          ys = this.ys,
          c1s = this.c1s,
          c2s = this.c2s,
          c3s = this.c3s; // The rightmost point in the dataset should give an exact result

      var i = xs.length - 1;

      if (x === xs[i]) {
        return ys[i];
      } // Search for the interval x is in, returning the corresponding y if x is one of the original xs


      var low = 0;
      var high = c3s.length - 1;
      var mid;

      while (low <= high) {
        mid = Math.floor(0.5 * (low + high));
        var xHere = xs[mid];

        if (xHere < x) {
          low = mid + 1;
        } else if (xHere > x) {
          high = mid - 1;
        } else {
          return ys[mid];
        }
      }

      i = Math.max(0, high); // Interpolate

      var diff = x - xs[i];
      var diffSq = diff * diff;
      return ys[i] + c1s[i] * diff + c2s[i] * diffSq + c3s[i] * diff * diffSq;
    }
  }]);

  return MonotonicInterpolant;
}();
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/OffsetUtils.js


var ELEMENT_NODE = 1;
function getNodeClientOffset(node) {
  var el = node.nodeType === ELEMENT_NODE ? node : node.parentElement;

  if (!el) {
    return null;
  }

  var _el$getBoundingClient = el.getBoundingClientRect(),
      top = _el$getBoundingClient.top,
      left = _el$getBoundingClient.left;

  return {
    x: left,
    y: top
  };
}
function getEventClientOffset(e) {
  return {
    x: e.clientX,
    y: e.clientY
  };
}

function isImageNode(node) {
  var _document$documentEle;

  return node.nodeName === 'IMG' && (isFirefox() || !((_document$documentEle = document.documentElement) !== null && _document$documentEle !== void 0 && _document$documentEle.contains(node)));
}

function getDragPreviewSize(isImage, dragPreview, sourceWidth, sourceHeight) {
  var dragPreviewWidth = isImage ? dragPreview.width : sourceWidth;
  var dragPreviewHeight = isImage ? dragPreview.height : sourceHeight; // Work around @2x coordinate discrepancies in browsers

  if (isSafari() && isImage) {
    dragPreviewHeight /= window.devicePixelRatio;
    dragPreviewWidth /= window.devicePixelRatio;
  }

  return {
    dragPreviewWidth: dragPreviewWidth,
    dragPreviewHeight: dragPreviewHeight
  };
}

function getDragPreviewOffset(sourceNode, dragPreview, clientOffset, anchorPoint, offsetPoint) {
  // The browsers will use the image intrinsic size under different conditions.
  // Firefox only cares if it's an image, but WebKit also wants it to be detached.
  var isImage = isImageNode(dragPreview);
  var dragPreviewNode = isImage ? sourceNode : dragPreview;
  var dragPreviewNodeOffsetFromClient = getNodeClientOffset(dragPreviewNode);
  var offsetFromDragPreview = {
    x: clientOffset.x - dragPreviewNodeOffsetFromClient.x,
    y: clientOffset.y - dragPreviewNodeOffsetFromClient.y
  };
  var sourceWidth = sourceNode.offsetWidth,
      sourceHeight = sourceNode.offsetHeight;
  var anchorX = anchorPoint.anchorX,
      anchorY = anchorPoint.anchorY;

  var _getDragPreviewSize = getDragPreviewSize(isImage, dragPreview, sourceWidth, sourceHeight),
      dragPreviewWidth = _getDragPreviewSize.dragPreviewWidth,
      dragPreviewHeight = _getDragPreviewSize.dragPreviewHeight;

  var calculateYOffset = function calculateYOffset() {
    var interpolantY = new MonotonicInterpolant([0, 0.5, 1], [// Dock to the top
    offsetFromDragPreview.y, // Align at the center
    offsetFromDragPreview.y / sourceHeight * dragPreviewHeight, // Dock to the bottom
    offsetFromDragPreview.y + dragPreviewHeight - sourceHeight]);
    var y = interpolantY.interpolate(anchorY); // Work around Safari 8 positioning bug

    if (isSafari() && isImage) {
      // We'll have to wait for @3x to see if this is entirely correct
      y += (window.devicePixelRatio - 1) * dragPreviewHeight;
    }

    return y;
  };

  var calculateXOffset = function calculateXOffset() {
    // Interpolate coordinates depending on anchor point
    // If you know a simpler way to do this, let me know
    var interpolantX = new MonotonicInterpolant([0, 0.5, 1], [// Dock to the left
    offsetFromDragPreview.x, // Align at the center
    offsetFromDragPreview.x / sourceWidth * dragPreviewWidth, // Dock to the right
    offsetFromDragPreview.x + dragPreviewWidth - sourceWidth]);
    return interpolantX.interpolate(anchorX);
  }; // Force offsets if specified in the options.


  var offsetX = offsetPoint.offsetX,
      offsetY = offsetPoint.offsetY;
  var isManualOffsetX = offsetX === 0 || offsetX;
  var isManualOffsetY = offsetY === 0 || offsetY;
  return {
    x: isManualOffsetX ? offsetX : calculateXOffset(),
    y: isManualOffsetY ? offsetY : calculateYOffset()
  };
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/NativeTypes.js
var FILE = '__NATIVE_FILE__';
var URL = '__NATIVE_URL__';
var TEXT = '__NATIVE_TEXT__';
var HTML = '__NATIVE_HTML__';
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/NativeDragSources/getDataFromDataTransfer.js
function getDataFromDataTransfer(dataTransfer, typesToTry, defaultValue) {
  var result = typesToTry.reduce(function (resultSoFar, typeToTry) {
    return resultSoFar || dataTransfer.getData(typeToTry);
  }, '');
  return result != null ? result : defaultValue;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/NativeDragSources/nativeTypesConfig.js
var _nativeTypesConfig;

function nativeTypesConfig_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }



var nativeTypesConfig = (_nativeTypesConfig = {}, nativeTypesConfig_defineProperty(_nativeTypesConfig, FILE, {
  exposeProperties: {
    files: function files(dataTransfer) {
      return Array.prototype.slice.call(dataTransfer.files);
    },
    items: function items(dataTransfer) {
      return dataTransfer.items;
    },
    dataTransfer: function dataTransfer(_dataTransfer) {
      return _dataTransfer;
    }
  },
  matchesTypes: ['Files']
}), nativeTypesConfig_defineProperty(_nativeTypesConfig, HTML, {
  exposeProperties: {
    html: function html(dataTransfer, matchesTypes) {
      return getDataFromDataTransfer(dataTransfer, matchesTypes, '');
    },
    dataTransfer: function dataTransfer(_dataTransfer2) {
      return _dataTransfer2;
    }
  },
  matchesTypes: ['Html', 'text/html']
}), nativeTypesConfig_defineProperty(_nativeTypesConfig, URL, {
  exposeProperties: {
    urls: function urls(dataTransfer, matchesTypes) {
      return getDataFromDataTransfer(dataTransfer, matchesTypes, '').split('\n');
    },
    dataTransfer: function dataTransfer(_dataTransfer3) {
      return _dataTransfer3;
    }
  },
  matchesTypes: ['Url', 'text/uri-list']
}), nativeTypesConfig_defineProperty(_nativeTypesConfig, TEXT, {
  exposeProperties: {
    text: function text(dataTransfer, matchesTypes) {
      return getDataFromDataTransfer(dataTransfer, matchesTypes, '');
    },
    dataTransfer: function dataTransfer(_dataTransfer4) {
      return _dataTransfer4;
    }
  },
  matchesTypes: ['Text', 'text/plain']
}), _nativeTypesConfig);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/NativeDragSources/NativeDragSource.js
function NativeDragSource_classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function NativeDragSource_defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function NativeDragSource_createClass(Constructor, protoProps, staticProps) { if (protoProps) NativeDragSource_defineProperties(Constructor.prototype, protoProps); if (staticProps) NativeDragSource_defineProperties(Constructor, staticProps); return Constructor; }

function NativeDragSource_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }

var NativeDragSource = /*#__PURE__*/function () {
  function NativeDragSource(config) {
    NativeDragSource_classCallCheck(this, NativeDragSource);

    NativeDragSource_defineProperty(this, "item", void 0);

    NativeDragSource_defineProperty(this, "config", void 0);

    this.config = config;
    this.item = {};
    this.initializeExposedProperties();
  }

  NativeDragSource_createClass(NativeDragSource, [{
    key: "initializeExposedProperties",
    value: function initializeExposedProperties() {
      var _this = this;

      Object.keys(this.config.exposeProperties).forEach(function (property) {
        Object.defineProperty(_this.item, property, {
          configurable: true,
          enumerable: true,
          get: function get() {
            // eslint-disable-next-line no-console
            console.warn("Browser doesn't allow reading \"".concat(property, "\" until the drop event."));
            return null;
          }
        });
      });
    }
  }, {
    key: "loadDataTransfer",
    value: function loadDataTransfer(dataTransfer) {
      var _this2 = this;

      if (dataTransfer) {
        var newProperties = {};
        Object.keys(this.config.exposeProperties).forEach(function (property) {
          newProperties[property] = {
            value: _this2.config.exposeProperties[property](dataTransfer, _this2.config.matchesTypes),
            configurable: true,
            enumerable: true
          };
        });
        Object.defineProperties(this.item, newProperties);
      }
    }
  }, {
    key: "canDrag",
    value: function canDrag() {
      return true;
    }
  }, {
    key: "beginDrag",
    value: function beginDrag() {
      return this.item;
    }
  }, {
    key: "isDragging",
    value: function isDragging(monitor, handle) {
      return handle === monitor.getSourceId();
    }
  }, {
    key: "endDrag",
    value: function endDrag() {// empty
    }
  }]);

  return NativeDragSource;
}();
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/NativeDragSources/index.js


function createNativeDragSource(type, dataTransfer) {
  var result = new NativeDragSource(nativeTypesConfig[type]);
  result.loadDataTransfer(dataTransfer);
  return result;
}
function matchNativeItemType(dataTransfer) {
  if (!dataTransfer) {
    return null;
  }

  var dataTransferTypes = Array.prototype.slice.call(dataTransfer.types || []);
  return Object.keys(nativeTypesConfig).filter(function (nativeItemType) {
    var matchesTypes = nativeTypesConfig[nativeItemType].matchesTypes;
    return matchesTypes.some(function (t) {
      return dataTransferTypes.indexOf(t) > -1;
    });
  })[0] || null;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/OptionsReader.js
function OptionsReader_classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function OptionsReader_defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function OptionsReader_createClass(Constructor, protoProps, staticProps) { if (protoProps) OptionsReader_defineProperties(Constructor.prototype, protoProps); if (staticProps) OptionsReader_defineProperties(Constructor, staticProps); return Constructor; }

function OptionsReader_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }

var OptionsReader = /*#__PURE__*/function () {
  function OptionsReader(globalContext, options) {
    OptionsReader_classCallCheck(this, OptionsReader);

    OptionsReader_defineProperty(this, "ownerDocument", null);

    OptionsReader_defineProperty(this, "globalContext", void 0);

    OptionsReader_defineProperty(this, "optionsArgs", void 0);

    this.globalContext = globalContext;
    this.optionsArgs = options;
  }

  OptionsReader_createClass(OptionsReader, [{
    key: "window",
    get: function get() {
      if (this.globalContext) {
        return this.globalContext;
      } else if (typeof window !== 'undefined') {
        return window;
      }

      return undefined;
    }
  }, {
    key: "document",
    get: function get() {
      var _this$globalContext;

      if ((_this$globalContext = this.globalContext) !== null && _this$globalContext !== void 0 && _this$globalContext.document) {
        return this.globalContext.document;
      } else if (this.window) {
        return this.window.document;
      } else {
        return undefined;
      }
    }
  }, {
    key: "rootElement",
    get: function get() {
      var _this$optionsArgs;

      return ((_this$optionsArgs = this.optionsArgs) === null || _this$optionsArgs === void 0 ? void 0 : _this$optionsArgs.rootElement) || this.window;
    }
  }]);

  return OptionsReader;
}();
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/HTML5BackendImpl.js
function ownKeys(object, enumerableOnly) { var keys = Object.keys(object); if (Object.getOwnPropertySymbols) { var symbols = Object.getOwnPropertySymbols(object); if (enumerableOnly) { symbols = symbols.filter(function (sym) { return Object.getOwnPropertyDescriptor(object, sym).enumerable; }); } keys.push.apply(keys, symbols); } return keys; }

function HTML5BackendImpl_objectSpread(target) { for (var i = 1; i < arguments.length; i++) { var source = arguments[i] != null ? arguments[i] : {}; if (i % 2) { ownKeys(Object(source), true).forEach(function (key) { HTML5BackendImpl_defineProperty(target, key, source[key]); }); } else if (Object.getOwnPropertyDescriptors) { Object.defineProperties(target, Object.getOwnPropertyDescriptors(source)); } else { ownKeys(Object(source)).forEach(function (key) { Object.defineProperty(target, key, Object.getOwnPropertyDescriptor(source, key)); }); } } return target; }

function HTML5BackendImpl_classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function HTML5BackendImpl_defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function HTML5BackendImpl_createClass(Constructor, protoProps, staticProps) { if (protoProps) HTML5BackendImpl_defineProperties(Constructor.prototype, protoProps); if (staticProps) HTML5BackendImpl_defineProperties(Constructor, staticProps); return Constructor; }

function HTML5BackendImpl_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }






var HTML5BackendImpl = /*#__PURE__*/function () {
  // React-Dnd Components
  // Internal State
  function HTML5BackendImpl(manager, globalContext, options) {
    var _this = this;

    HTML5BackendImpl_classCallCheck(this, HTML5BackendImpl);

    HTML5BackendImpl_defineProperty(this, "options", void 0);

    HTML5BackendImpl_defineProperty(this, "actions", void 0);

    HTML5BackendImpl_defineProperty(this, "monitor", void 0);

    HTML5BackendImpl_defineProperty(this, "registry", void 0);

    HTML5BackendImpl_defineProperty(this, "enterLeaveCounter", void 0);

    HTML5BackendImpl_defineProperty(this, "sourcePreviewNodes", new Map());

    HTML5BackendImpl_defineProperty(this, "sourcePreviewNodeOptions", new Map());

    HTML5BackendImpl_defineProperty(this, "sourceNodes", new Map());

    HTML5BackendImpl_defineProperty(this, "sourceNodeOptions", new Map());

    HTML5BackendImpl_defineProperty(this, "dragStartSourceIds", null);

    HTML5BackendImpl_defineProperty(this, "dropTargetIds", []);

    HTML5BackendImpl_defineProperty(this, "dragEnterTargetIds", []);

    HTML5BackendImpl_defineProperty(this, "currentNativeSource", null);

    HTML5BackendImpl_defineProperty(this, "currentNativeHandle", null);

    HTML5BackendImpl_defineProperty(this, "currentDragSourceNode", null);

    HTML5BackendImpl_defineProperty(this, "altKeyPressed", false);

    HTML5BackendImpl_defineProperty(this, "mouseMoveTimeoutTimer", null);

    HTML5BackendImpl_defineProperty(this, "asyncEndDragFrameId", null);

    HTML5BackendImpl_defineProperty(this, "dragOverTargetIds", null);

    HTML5BackendImpl_defineProperty(this, "lastClientOffset", null);

    HTML5BackendImpl_defineProperty(this, "hoverRafId", null);

    HTML5BackendImpl_defineProperty(this, "getSourceClientOffset", function (sourceId) {
      var source = _this.sourceNodes.get(sourceId);

      return source && getNodeClientOffset(source) || null;
    });

    HTML5BackendImpl_defineProperty(this, "endDragNativeItem", function () {
      if (!_this.isDraggingNativeItem()) {
        return;
      }

      _this.actions.endDrag();

      if (_this.currentNativeHandle) {
        _this.registry.removeSource(_this.currentNativeHandle);
      }

      _this.currentNativeHandle = null;
      _this.currentNativeSource = null;
    });

    HTML5BackendImpl_defineProperty(this, "isNodeInDocument", function (node) {
      // Check the node either in the main document or in the current context
      return Boolean(node && _this.document && _this.document.body && _this.document.body.contains(node));
    });

    HTML5BackendImpl_defineProperty(this, "endDragIfSourceWasRemovedFromDOM", function () {
      var node = _this.currentDragSourceNode;

      if (node == null || _this.isNodeInDocument(node)) {
        return;
      }

      if (_this.clearCurrentDragSourceNode() && _this.monitor.isDragging()) {
        _this.actions.endDrag();
      }
    });

    HTML5BackendImpl_defineProperty(this, "handleTopDragStartCapture", function () {
      _this.clearCurrentDragSourceNode();

      _this.dragStartSourceIds = [];
    });

    HTML5BackendImpl_defineProperty(this, "handleTopDragStart", function (e) {
      if (e.defaultPrevented) {
        return;
      }

      var dragStartSourceIds = _this.dragStartSourceIds;
      _this.dragStartSourceIds = null;
      var clientOffset = getEventClientOffset(e); // Avoid crashing if we missed a drop event or our previous drag died

      if (_this.monitor.isDragging()) {
        _this.actions.endDrag();
      } // Don't publish the source just yet (see why below)


      _this.actions.beginDrag(dragStartSourceIds || [], {
        publishSource: false,
        getSourceClientOffset: _this.getSourceClientOffset,
        clientOffset: clientOffset
      });

      var dataTransfer = e.dataTransfer;
      var nativeType = matchNativeItemType(dataTransfer);

      if (_this.monitor.isDragging()) {
        if (dataTransfer && typeof dataTransfer.setDragImage === 'function') {
          // Use custom drag image if user specifies it.
          // If child drag source refuses drag but parent agrees,
          // use parent's node as drag image. Neither works in IE though.
          var sourceId = _this.monitor.getSourceId();

          var sourceNode = _this.sourceNodes.get(sourceId);

          var dragPreview = _this.sourcePreviewNodes.get(sourceId) || sourceNode;

          if (dragPreview) {
            var _this$getCurrentSourc = _this.getCurrentSourcePreviewNodeOptions(),
                anchorX = _this$getCurrentSourc.anchorX,
                anchorY = _this$getCurrentSourc.anchorY,
                offsetX = _this$getCurrentSourc.offsetX,
                offsetY = _this$getCurrentSourc.offsetY;

            var anchorPoint = {
              anchorX: anchorX,
              anchorY: anchorY
            };
            var offsetPoint = {
              offsetX: offsetX,
              offsetY: offsetY
            };
            var dragPreviewOffset = getDragPreviewOffset(sourceNode, dragPreview, clientOffset, anchorPoint, offsetPoint);
            dataTransfer.setDragImage(dragPreview, dragPreviewOffset.x, dragPreviewOffset.y);
          }
        }

        try {
          // Firefox won't drag without setting data
          dataTransfer === null || dataTransfer === void 0 ? void 0 : dataTransfer.setData('application/json', {});
        } catch (err) {// IE doesn't support MIME types in setData
        } // Store drag source node so we can check whether
        // it is removed from DOM and trigger endDrag manually.


        _this.setCurrentDragSourceNode(e.target); // Now we are ready to publish the drag source.. or are we not?


        var _this$getCurrentSourc2 = _this.getCurrentSourcePreviewNodeOptions(),
            captureDraggingState = _this$getCurrentSourc2.captureDraggingState;

        if (!captureDraggingState) {
          // Usually we want to publish it in the next tick so that browser
          // is able to screenshot the current (not yet dragging) state.
          //
          // It also neatly avoids a situation where render() returns null
          // in the same tick for the source element, and browser freaks out.
          setTimeout(function () {
            return _this.actions.publishDragSource();
          }, 0);
        } else {
          // In some cases the user may want to override this behavior, e.g.
          // to work around IE not supporting custom drag previews.
          //
          // When using a custom drag layer, the only way to prevent
          // the default drag preview from drawing in IE is to screenshot
          // the dragging state in which the node itself has zero opacity
          // and height. In this case, though, returning null from render()
          // will abruptly end the dragging, which is not obvious.
          //
          // This is the reason such behavior is strictly opt-in.
          _this.actions.publishDragSource();
        }
      } else if (nativeType) {
        // A native item (such as URL) dragged from inside the document
        _this.beginDragNativeItem(nativeType);
      } else if (dataTransfer && !dataTransfer.types && (e.target && !e.target.hasAttribute || !e.target.hasAttribute('draggable'))) {
        // Looks like a Safari bug: dataTransfer.types is null, but there was no draggable.
        // Just let it drag. It's a native type (URL or text) and will be picked up in
        // dragenter handler.
        return;
      } else {
        // If by this time no drag source reacted, tell browser not to drag.
        e.preventDefault();
      }
    });

    HTML5BackendImpl_defineProperty(this, "handleTopDragEndCapture", function () {
      if (_this.clearCurrentDragSourceNode() && _this.monitor.isDragging()) {
        // Firefox can dispatch this event in an infinite loop
        // if dragend handler does something like showing an alert.
        // Only proceed if we have not handled it already.
        _this.actions.endDrag();
      }
    });

    HTML5BackendImpl_defineProperty(this, "handleTopDragEnterCapture", function (e) {
      _this.dragEnterTargetIds = [];

      var isFirstEnter = _this.enterLeaveCounter.enter(e.target);

      if (!isFirstEnter || _this.monitor.isDragging()) {
        return;
      }

      var dataTransfer = e.dataTransfer;
      var nativeType = matchNativeItemType(dataTransfer);

      if (nativeType) {
        // A native item (such as file or URL) dragged from outside the document
        _this.beginDragNativeItem(nativeType, dataTransfer);
      }
    });

    HTML5BackendImpl_defineProperty(this, "handleTopDragEnter", function (e) {
      var dragEnterTargetIds = _this.dragEnterTargetIds;
      _this.dragEnterTargetIds = [];

      if (!_this.monitor.isDragging()) {
        // This is probably a native item type we don't understand.
        return;
      }

      _this.altKeyPressed = e.altKey; // If the target changes position as the result of `dragenter`, `dragover` might still
      // get dispatched despite target being no longer there. The easy solution is to check
      // whether there actually is a target before firing `hover`.

      if (dragEnterTargetIds.length > 0) {
        _this.actions.hover(dragEnterTargetIds, {
          clientOffset: getEventClientOffset(e)
        });
      }

      var canDrop = dragEnterTargetIds.some(function (targetId) {
        return _this.monitor.canDropOnTarget(targetId);
      });

      if (canDrop) {
        // IE requires this to fire dragover events
        e.preventDefault();

        if (e.dataTransfer) {
          e.dataTransfer.dropEffect = _this.getCurrentDropEffect();
        }
      }
    });

    HTML5BackendImpl_defineProperty(this, "handleTopDragOverCapture", function () {
      _this.dragOverTargetIds = [];
    });

    HTML5BackendImpl_defineProperty(this, "handleTopDragOver", function (e) {
      var dragOverTargetIds = _this.dragOverTargetIds;
      _this.dragOverTargetIds = [];

      if (!_this.monitor.isDragging()) {
        // This is probably a native item type we don't understand.
        // Prevent default "drop and blow away the whole document" action.
        e.preventDefault();

        if (e.dataTransfer) {
          e.dataTransfer.dropEffect = 'none';
        }

        return;
      }

      _this.altKeyPressed = e.altKey;
      _this.lastClientOffset = getEventClientOffset(e);

      if (_this.hoverRafId === null && typeof requestAnimationFrame !== 'undefined') {
        _this.hoverRafId = requestAnimationFrame(function () {
          if (_this.monitor.isDragging()) {
            _this.actions.hover(dragOverTargetIds || [], {
              clientOffset: _this.lastClientOffset
            });
          }

          _this.hoverRafId = null;
        });
      }

      var canDrop = (dragOverTargetIds || []).some(function (targetId) {
        return _this.monitor.canDropOnTarget(targetId);
      });

      if (canDrop) {
        // Show user-specified drop effect.
        e.preventDefault();

        if (e.dataTransfer) {
          e.dataTransfer.dropEffect = _this.getCurrentDropEffect();
        }
      } else if (_this.isDraggingNativeItem()) {
        // Don't show a nice cursor but still prevent default
        // "drop and blow away the whole document" action.
        e.preventDefault();
      } else {
        e.preventDefault();

        if (e.dataTransfer) {
          e.dataTransfer.dropEffect = 'none';
        }
      }
    });

    HTML5BackendImpl_defineProperty(this, "handleTopDragLeaveCapture", function (e) {
      if (_this.isDraggingNativeItem()) {
        e.preventDefault();
      }

      var isLastLeave = _this.enterLeaveCounter.leave(e.target);

      if (!isLastLeave) {
        return;
      }

      if (_this.isDraggingNativeItem()) {
        setTimeout(function () {
          return _this.endDragNativeItem();
        }, 0);
      }
    });

    HTML5BackendImpl_defineProperty(this, "handleTopDropCapture", function (e) {
      _this.dropTargetIds = [];

      if (_this.isDraggingNativeItem()) {
        var _this$currentNativeSo;

        e.preventDefault();
        (_this$currentNativeSo = _this.currentNativeSource) === null || _this$currentNativeSo === void 0 ? void 0 : _this$currentNativeSo.loadDataTransfer(e.dataTransfer);
      } else if (matchNativeItemType(e.dataTransfer)) {
        // Dragging some elements, like <a> and <img> may still behave like a native drag event,
        // even if the current drag event matches a user-defined type.
        // Stop the default behavior when we're not expecting a native item to be dropped.
        e.preventDefault();
      }

      _this.enterLeaveCounter.reset();
    });

    HTML5BackendImpl_defineProperty(this, "handleTopDrop", function (e) {
      var dropTargetIds = _this.dropTargetIds;
      _this.dropTargetIds = [];

      _this.actions.hover(dropTargetIds, {
        clientOffset: getEventClientOffset(e)
      });

      _this.actions.drop({
        dropEffect: _this.getCurrentDropEffect()
      });

      if (_this.isDraggingNativeItem()) {
        _this.endDragNativeItem();
      } else if (_this.monitor.isDragging()) {
        _this.actions.endDrag();
      }
    });

    HTML5BackendImpl_defineProperty(this, "handleSelectStart", function (e) {
      var target = e.target; // Only IE requires us to explicitly say
      // we want drag drop operation to start

      if (typeof target.dragDrop !== 'function') {
        return;
      } // Inputs and textareas should be selectable


      if (target.tagName === 'INPUT' || target.tagName === 'SELECT' || target.tagName === 'TEXTAREA' || target.isContentEditable) {
        return;
      } // For other targets, ask IE
      // to enable drag and drop


      e.preventDefault();
      target.dragDrop();
    });

    this.options = new OptionsReader(globalContext, options);
    this.actions = manager.getActions();
    this.monitor = manager.getMonitor();
    this.registry = manager.getRegistry();
    this.enterLeaveCounter = new EnterLeaveCounter(this.isNodeInDocument);
  }
  /**
   * Generate profiling statistics for the HTML5Backend.
   */


  HTML5BackendImpl_createClass(HTML5BackendImpl, [{
    key: "profile",
    value: function profile() {
      var _this$dragStartSource, _this$dragOverTargetI;

      return {
        sourcePreviewNodes: this.sourcePreviewNodes.size,
        sourcePreviewNodeOptions: this.sourcePreviewNodeOptions.size,
        sourceNodeOptions: this.sourceNodeOptions.size,
        sourceNodes: this.sourceNodes.size,
        dragStartSourceIds: ((_this$dragStartSource = this.dragStartSourceIds) === null || _this$dragStartSource === void 0 ? void 0 : _this$dragStartSource.length) || 0,
        dropTargetIds: this.dropTargetIds.length,
        dragEnterTargetIds: this.dragEnterTargetIds.length,
        dragOverTargetIds: ((_this$dragOverTargetI = this.dragOverTargetIds) === null || _this$dragOverTargetI === void 0 ? void 0 : _this$dragOverTargetI.length) || 0
      };
    } // public for test

  }, {
    key: "window",
    get: function get() {
      return this.options.window;
    }
  }, {
    key: "document",
    get: function get() {
      return this.options.document;
    }
    /**
     * Get the root element to use for event subscriptions
     */

  }, {
    key: "rootElement",
    get: function get() {
      return this.options.rootElement;
    }
  }, {
    key: "setup",
    value: function setup() {
      var root = this.rootElement;

      if (root === undefined) {
        return;
      }

      if (root.__isReactDndBackendSetUp) {
        throw new Error('Cannot have two HTML5 backends at the same time.');
      }

      root.__isReactDndBackendSetUp = true;
      this.addEventListeners(root);
    }
  }, {
    key: "teardown",
    value: function teardown() {
      var root = this.rootElement;

      if (root === undefined) {
        return;
      }

      root.__isReactDndBackendSetUp = false;
      this.removeEventListeners(this.rootElement);
      this.clearCurrentDragSourceNode();

      if (this.asyncEndDragFrameId) {
        var _this$window;

        (_this$window = this.window) === null || _this$window === void 0 ? void 0 : _this$window.cancelAnimationFrame(this.asyncEndDragFrameId);
      }
    }
  }, {
    key: "connectDragPreview",
    value: function connectDragPreview(sourceId, node, options) {
      var _this2 = this;

      this.sourcePreviewNodeOptions.set(sourceId, options);
      this.sourcePreviewNodes.set(sourceId, node);
      return function () {
        _this2.sourcePreviewNodes.delete(sourceId);

        _this2.sourcePreviewNodeOptions.delete(sourceId);
      };
    }
  }, {
    key: "connectDragSource",
    value: function connectDragSource(sourceId, node, options) {
      var _this3 = this;

      this.sourceNodes.set(sourceId, node);
      this.sourceNodeOptions.set(sourceId, options);

      var handleDragStart = function handleDragStart(e) {
        return _this3.handleDragStart(e, sourceId);
      };

      var handleSelectStart = function handleSelectStart(e) {
        return _this3.handleSelectStart(e);
      };

      node.setAttribute('draggable', 'true');
      node.addEventListener('dragstart', handleDragStart);
      node.addEventListener('selectstart', handleSelectStart);
      return function () {
        _this3.sourceNodes.delete(sourceId);

        _this3.sourceNodeOptions.delete(sourceId);

        node.removeEventListener('dragstart', handleDragStart);
        node.removeEventListener('selectstart', handleSelectStart);
        node.setAttribute('draggable', 'false');
      };
    }
  }, {
    key: "connectDropTarget",
    value: function connectDropTarget(targetId, node) {
      var _this4 = this;

      var handleDragEnter = function handleDragEnter(e) {
        return _this4.handleDragEnter(e, targetId);
      };

      var handleDragOver = function handleDragOver(e) {
        return _this4.handleDragOver(e, targetId);
      };

      var handleDrop = function handleDrop(e) {
        return _this4.handleDrop(e, targetId);
      };

      node.addEventListener('dragenter', handleDragEnter);
      node.addEventListener('dragover', handleDragOver);
      node.addEventListener('drop', handleDrop);
      return function () {
        node.removeEventListener('dragenter', handleDragEnter);
        node.removeEventListener('dragover', handleDragOver);
        node.removeEventListener('drop', handleDrop);
      };
    }
  }, {
    key: "addEventListeners",
    value: function addEventListeners(target) {
      // SSR Fix (https://github.com/react-dnd/react-dnd/pull/813
      if (!target.addEventListener) {
        return;
      }

      target.addEventListener('dragstart', this.handleTopDragStart);
      target.addEventListener('dragstart', this.handleTopDragStartCapture, true);
      target.addEventListener('dragend', this.handleTopDragEndCapture, true);
      target.addEventListener('dragenter', this.handleTopDragEnter);
      target.addEventListener('dragenter', this.handleTopDragEnterCapture, true);
      target.addEventListener('dragleave', this.handleTopDragLeaveCapture, true);
      target.addEventListener('dragover', this.handleTopDragOver);
      target.addEventListener('dragover', this.handleTopDragOverCapture, true);
      target.addEventListener('drop', this.handleTopDrop);
      target.addEventListener('drop', this.handleTopDropCapture, true);
    }
  }, {
    key: "removeEventListeners",
    value: function removeEventListeners(target) {
      // SSR Fix (https://github.com/react-dnd/react-dnd/pull/813
      if (!target.removeEventListener) {
        return;
      }

      target.removeEventListener('dragstart', this.handleTopDragStart);
      target.removeEventListener('dragstart', this.handleTopDragStartCapture, true);
      target.removeEventListener('dragend', this.handleTopDragEndCapture, true);
      target.removeEventListener('dragenter', this.handleTopDragEnter);
      target.removeEventListener('dragenter', this.handleTopDragEnterCapture, true);
      target.removeEventListener('dragleave', this.handleTopDragLeaveCapture, true);
      target.removeEventListener('dragover', this.handleTopDragOver);
      target.removeEventListener('dragover', this.handleTopDragOverCapture, true);
      target.removeEventListener('drop', this.handleTopDrop);
      target.removeEventListener('drop', this.handleTopDropCapture, true);
    }
  }, {
    key: "getCurrentSourceNodeOptions",
    value: function getCurrentSourceNodeOptions() {
      var sourceId = this.monitor.getSourceId();
      var sourceNodeOptions = this.sourceNodeOptions.get(sourceId);
      return HTML5BackendImpl_objectSpread({
        dropEffect: this.altKeyPressed ? 'copy' : 'move'
      }, sourceNodeOptions || {});
    }
  }, {
    key: "getCurrentDropEffect",
    value: function getCurrentDropEffect() {
      if (this.isDraggingNativeItem()) {
        // It makes more sense to default to 'copy' for native resources
        return 'copy';
      }

      return this.getCurrentSourceNodeOptions().dropEffect;
    }
  }, {
    key: "getCurrentSourcePreviewNodeOptions",
    value: function getCurrentSourcePreviewNodeOptions() {
      var sourceId = this.monitor.getSourceId();
      var sourcePreviewNodeOptions = this.sourcePreviewNodeOptions.get(sourceId);
      return HTML5BackendImpl_objectSpread({
        anchorX: 0.5,
        anchorY: 0.5,
        captureDraggingState: false
      }, sourcePreviewNodeOptions || {});
    }
  }, {
    key: "isDraggingNativeItem",
    value: function isDraggingNativeItem() {
      var itemType = this.monitor.getItemType();
      return Object.keys(NativeTypes_namespaceObject).some(function (key) {
        return NativeTypes_namespaceObject[key] === itemType;
      });
    }
  }, {
    key: "beginDragNativeItem",
    value: function beginDragNativeItem(type, dataTransfer) {
      this.clearCurrentDragSourceNode();
      this.currentNativeSource = createNativeDragSource(type, dataTransfer);
      this.currentNativeHandle = this.registry.addSource(type, this.currentNativeSource);
      this.actions.beginDrag([this.currentNativeHandle]);
    }
  }, {
    key: "setCurrentDragSourceNode",
    value: function setCurrentDragSourceNode(node) {
      var _this5 = this;

      this.clearCurrentDragSourceNode();
      this.currentDragSourceNode = node; // A timeout of > 0 is necessary to resolve Firefox issue referenced
      // See:
      //   * https://github.com/react-dnd/react-dnd/pull/928
      //   * https://github.com/react-dnd/react-dnd/issues/869

      var MOUSE_MOVE_TIMEOUT = 1000; // Receiving a mouse event in the middle of a dragging operation
      // means it has ended and the drag source node disappeared from DOM,
      // so the browser didn't dispatch the dragend event.
      //
      // We need to wait before we start listening for mousemove events.
      // This is needed because the drag preview needs to be drawn or else it fires an 'mousemove' event
      // immediately in some browsers.
      //
      // See:
      //   * https://github.com/react-dnd/react-dnd/pull/928
      //   * https://github.com/react-dnd/react-dnd/issues/869
      //

      this.mouseMoveTimeoutTimer = setTimeout(function () {
        var _this5$rootElement;

        return (_this5$rootElement = _this5.rootElement) === null || _this5$rootElement === void 0 ? void 0 : _this5$rootElement.addEventListener('mousemove', _this5.endDragIfSourceWasRemovedFromDOM, true);
      }, MOUSE_MOVE_TIMEOUT);
    }
  }, {
    key: "clearCurrentDragSourceNode",
    value: function clearCurrentDragSourceNode() {
      if (this.currentDragSourceNode) {
        this.currentDragSourceNode = null;

        if (this.rootElement) {
          var _this$window2;

          (_this$window2 = this.window) === null || _this$window2 === void 0 ? void 0 : _this$window2.clearTimeout(this.mouseMoveTimeoutTimer || undefined);
          this.rootElement.removeEventListener('mousemove', this.endDragIfSourceWasRemovedFromDOM, true);
        }

        this.mouseMoveTimeoutTimer = null;
        return true;
      }

      return false;
    }
  }, {
    key: "handleDragStart",
    value: function handleDragStart(e, sourceId) {
      if (e.defaultPrevented) {
        return;
      }

      if (!this.dragStartSourceIds) {
        this.dragStartSourceIds = [];
      }

      this.dragStartSourceIds.unshift(sourceId);
    }
  }, {
    key: "handleDragEnter",
    value: function handleDragEnter(e, targetId) {
      this.dragEnterTargetIds.unshift(targetId);
    }
  }, {
    key: "handleDragOver",
    value: function handleDragOver(e, targetId) {
      if (this.dragOverTargetIds === null) {
        this.dragOverTargetIds = [];
      }

      this.dragOverTargetIds.unshift(targetId);
    }
  }, {
    key: "handleDrop",
    value: function handleDrop(e, targetId) {
      this.dropTargetIds.unshift(targetId);
    }
  }]);

  return HTML5BackendImpl;
}();
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd-html5-backend@14.1.0/node_modules/react-dnd-html5-backend/dist/esm/index.js




var HTML5Backend = function createBackend(manager, context, options) {
  return new HTML5BackendImpl(manager, context, options);
};
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/actions/dragDrop/types.js
var INIT_COORDS = 'dnd-core/INIT_COORDS';
var BEGIN_DRAG = 'dnd-core/BEGIN_DRAG';
var PUBLISH_DRAG_SOURCE = 'dnd-core/PUBLISH_DRAG_SOURCE';
var HOVER = 'dnd-core/HOVER';
var DROP = 'dnd-core/DROP';
var END_DRAG = 'dnd-core/END_DRAG';
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/actions/dragDrop/local/setClientOffset.js

function setClientOffset(clientOffset, sourceClientOffset) {
  return {
    type: INIT_COORDS,
    payload: {
      sourceClientOffset: sourceClientOffset || null,
      clientOffset: clientOffset || null
    }
  };
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/utils/js_utils.js
function js_utils_typeof(obj) { "@babel/helpers - typeof"; if (typeof Symbol === "function" && typeof Symbol.iterator === "symbol") { js_utils_typeof = function _typeof(obj) { return typeof obj; }; } else { js_utils_typeof = function _typeof(obj) { return obj && typeof Symbol === "function" && obj.constructor === Symbol && obj !== Symbol.prototype ? "symbol" : typeof obj; }; } return js_utils_typeof(obj); }

// cheap lodash replacements

/**
 * drop-in replacement for _.get
 * @param obj
 * @param path
 * @param defaultValue
 */
function get(obj, path, defaultValue) {
  return path.split('.').reduce(function (a, c) {
    return a && a[c] ? a[c] : defaultValue || null;
  }, obj);
}
/**
 * drop-in replacement for _.without
 */

function js_utils_without(items, item) {
  return items.filter(function (i) {
    return i !== item;
  });
}
/**
 * drop-in replacement for _.isString
 * @param input
 */

function isString(input) {
  return typeof input === 'string';
}
/**
 * drop-in replacement for _.isString
 * @param input
 */

function isObject(input) {
  return js_utils_typeof(input) === 'object';
}
/**
 * repalcement for _.xor
 * @param itemsA
 * @param itemsB
 */

function xor(itemsA, itemsB) {
  var map = new Map();

  var insertItem = function insertItem(item) {
    map.set(item, map.has(item) ? map.get(item) + 1 : 1);
  };

  itemsA.forEach(insertItem);
  itemsB.forEach(insertItem);
  var result = [];
  map.forEach(function (count, key) {
    if (count === 1) {
      result.push(key);
    }
  });
  return result;
}
/**
 * replacement for _.intersection
 * @param itemsA
 * @param itemsB
 */

function intersection(itemsA, itemsB) {
  return itemsA.filter(function (t) {
    return itemsB.indexOf(t) > -1;
  });
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/actions/dragDrop/beginDrag.js




var ResetCoordinatesAction = {
  type: INIT_COORDS,
  payload: {
    clientOffset: null,
    sourceClientOffset: null
  }
};
function createBeginDrag(manager) {
  return function beginDrag() {
    var sourceIds = arguments.length > 0 && arguments[0] !== undefined ? arguments[0] : [];
    var options = arguments.length > 1 && arguments[1] !== undefined ? arguments[1] : {
      publishSource: true
    };
    var _options$publishSourc = options.publishSource,
        publishSource = _options$publishSourc === void 0 ? true : _options$publishSourc,
        clientOffset = options.clientOffset,
        getSourceClientOffset = options.getSourceClientOffset;
    var monitor = manager.getMonitor();
    var registry = manager.getRegistry(); // Initialize the coordinates using the client offset

    manager.dispatch(setClientOffset(clientOffset));
    verifyInvariants(sourceIds, monitor, registry); // Get the draggable source

    var sourceId = getDraggableSource(sourceIds, monitor);

    if (sourceId === null) {
      manager.dispatch(ResetCoordinatesAction);
      return;
    } // Get the source client offset


    var sourceClientOffset = null;

    if (clientOffset) {
      if (!getSourceClientOffset) {
        throw new Error('getSourceClientOffset must be defined');
      }

      verifyGetSourceClientOffsetIsFunction(getSourceClientOffset);
      sourceClientOffset = getSourceClientOffset(sourceId);
    } // Initialize the full coordinates


    manager.dispatch(setClientOffset(clientOffset, sourceClientOffset));
    var source = registry.getSource(sourceId);
    var item = source.beginDrag(monitor, sourceId); // If source.beginDrag returns null, this is an indicator to cancel the drag

    if (item == null) {
      return undefined;
    }

    verifyItemIsObject(item);
    registry.pinSource(sourceId);
    var itemType = registry.getSourceType(sourceId);
    return {
      type: BEGIN_DRAG,
      payload: {
        itemType: itemType,
        item: item,
        sourceId: sourceId,
        clientOffset: clientOffset || null,
        sourceClientOffset: sourceClientOffset || null,
        isSourcePublic: !!publishSource
      }
    };
  };
}

function verifyInvariants(sourceIds, monitor, registry) {
  invariant(!monitor.isDragging(), 'Cannot call beginDrag while dragging.');
  sourceIds.forEach(function (sourceId) {
    invariant(registry.getSource(sourceId), 'Expected sourceIds to be registered.');
  });
}

function verifyGetSourceClientOffsetIsFunction(getSourceClientOffset) {
  invariant(typeof getSourceClientOffset === 'function', 'When clientOffset is provided, getSourceClientOffset must be a function.');
}

function verifyItemIsObject(item) {
  invariant(isObject(item), 'Item must be an object.');
}

function getDraggableSource(sourceIds, monitor) {
  var sourceId = null;

  for (var i = sourceIds.length - 1; i >= 0; i--) {
    if (monitor.canDragSource(sourceIds[i])) {
      sourceId = sourceIds[i];
      break;
    }
  }

  return sourceId;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/actions/dragDrop/publishDragSource.js

function createPublishDragSource(manager) {
  return function publishDragSource() {
    var monitor = manager.getMonitor();

    if (monitor.isDragging()) {
      return {
        type: PUBLISH_DRAG_SOURCE
      };
    }
  };
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/utils/matchesType.js
function matchesType(targetType, draggedItemType) {
  if (draggedItemType === null) {
    return targetType === null;
  }

  return Array.isArray(targetType) ? targetType.some(function (t) {
    return t === draggedItemType;
  }) : targetType === draggedItemType;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/actions/dragDrop/hover.js



function createHover(manager) {
  return function hover(targetIdsArg) {
    var _ref = arguments.length > 1 && arguments[1] !== undefined ? arguments[1] : {},
        clientOffset = _ref.clientOffset;

    verifyTargetIdsIsArray(targetIdsArg);
    var targetIds = targetIdsArg.slice(0);
    var monitor = manager.getMonitor();
    var registry = manager.getRegistry();
    checkInvariants(targetIds, monitor, registry);
    var draggedItemType = monitor.getItemType();
    removeNonMatchingTargetIds(targetIds, registry, draggedItemType);
    hoverAllTargets(targetIds, monitor, registry);
    return {
      type: HOVER,
      payload: {
        targetIds: targetIds,
        clientOffset: clientOffset || null
      }
    };
  };
}

function verifyTargetIdsIsArray(targetIdsArg) {
  invariant(Array.isArray(targetIdsArg), 'Expected targetIds to be an array.');
}

function checkInvariants(targetIds, monitor, registry) {
  invariant(monitor.isDragging(), 'Cannot call hover while not dragging.');
  invariant(!monitor.didDrop(), 'Cannot call hover after drop.');

  for (var i = 0; i < targetIds.length; i++) {
    var targetId = targetIds[i];
    invariant(targetIds.lastIndexOf(targetId) === i, 'Expected targetIds to be unique in the passed array.');
    var target = registry.getTarget(targetId);
    invariant(target, 'Expected targetIds to be registered.');
  }
}

function removeNonMatchingTargetIds(targetIds, registry, draggedItemType) {
  // Remove those targetIds that don't match the targetType.  This
  // fixes shallow isOver which would only be non-shallow because of
  // non-matching targets.
  for (var i = targetIds.length - 1; i >= 0; i--) {
    var targetId = targetIds[i];
    var targetType = registry.getTargetType(targetId);

    if (!matchesType(targetType, draggedItemType)) {
      targetIds.splice(i, 1);
    }
  }
}

function hoverAllTargets(targetIds, monitor, registry) {
  // Finally call hover on all matching targets.
  targetIds.forEach(function (targetId) {
    var target = registry.getTarget(targetId);
    target.hover(monitor, targetId);
  });
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/actions/dragDrop/drop.js
function drop_ownKeys(object, enumerableOnly) { var keys = Object.keys(object); if (Object.getOwnPropertySymbols) { var symbols = Object.getOwnPropertySymbols(object); if (enumerableOnly) { symbols = symbols.filter(function (sym) { return Object.getOwnPropertyDescriptor(object, sym).enumerable; }); } keys.push.apply(keys, symbols); } return keys; }

function drop_objectSpread(target) { for (var i = 1; i < arguments.length; i++) { var source = arguments[i] != null ? arguments[i] : {}; if (i % 2) { drop_ownKeys(Object(source), true).forEach(function (key) { drop_defineProperty(target, key, source[key]); }); } else if (Object.getOwnPropertyDescriptors) { Object.defineProperties(target, Object.getOwnPropertyDescriptors(source)); } else { drop_ownKeys(Object(source)).forEach(function (key) { Object.defineProperty(target, key, Object.getOwnPropertyDescriptor(source, key)); }); } } return target; }

function drop_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }




function createDrop(manager) {
  return function drop() {
    var options = arguments.length > 0 && arguments[0] !== undefined ? arguments[0] : {};
    var monitor = manager.getMonitor();
    var registry = manager.getRegistry();
    drop_verifyInvariants(monitor);
    var targetIds = getDroppableTargets(monitor); // Multiple actions are dispatched here, which is why this doesn't return an action

    targetIds.forEach(function (targetId, index) {
      var dropResult = determineDropResult(targetId, index, registry, monitor);
      var action = {
        type: DROP,
        payload: {
          dropResult: drop_objectSpread(drop_objectSpread({}, options), dropResult)
        }
      };
      manager.dispatch(action);
    });
  };
}

function drop_verifyInvariants(monitor) {
  invariant(monitor.isDragging(), 'Cannot call drop while not dragging.');
  invariant(!monitor.didDrop(), 'Cannot call drop twice during one drag operation.');
}

function determineDropResult(targetId, index, registry, monitor) {
  var target = registry.getTarget(targetId);
  var dropResult = target ? target.drop(monitor, targetId) : undefined;
  verifyDropResultType(dropResult);

  if (typeof dropResult === 'undefined') {
    dropResult = index === 0 ? {} : monitor.getDropResult();
  }

  return dropResult;
}

function verifyDropResultType(dropResult) {
  invariant(typeof dropResult === 'undefined' || isObject(dropResult), 'Drop result must either be an object or undefined.');
}

function getDroppableTargets(monitor) {
  var targetIds = monitor.getTargetIds().filter(monitor.canDropOnTarget, monitor);
  targetIds.reverse();
  return targetIds;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/actions/dragDrop/endDrag.js


function createEndDrag(manager) {
  return function endDrag() {
    var monitor = manager.getMonitor();
    var registry = manager.getRegistry();
    verifyIsDragging(monitor);
    var sourceId = monitor.getSourceId();

    if (sourceId != null) {
      var source = registry.getSource(sourceId, true);
      source.endDrag(monitor, sourceId);
      registry.unpinSource();
    }

    return {
      type: END_DRAG
    };
  };
}

function verifyIsDragging(monitor) {
  invariant(monitor.isDragging(), 'Cannot call endDrag while not dragging.');
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/actions/dragDrop/index.js






function createDragDropActions(manager) {
  return {
    beginDrag: createBeginDrag(manager),
    publishDragSource: createPublishDragSource(manager),
    hover: createHover(manager),
    drop: createDrop(manager),
    endDrag: createEndDrag(manager)
  };
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/classes/DragDropManagerImpl.js
function DragDropManagerImpl_classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function DragDropManagerImpl_defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function DragDropManagerImpl_createClass(Constructor, protoProps, staticProps) { if (protoProps) DragDropManagerImpl_defineProperties(Constructor.prototype, protoProps); if (staticProps) DragDropManagerImpl_defineProperties(Constructor, staticProps); return Constructor; }

function DragDropManagerImpl_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }


var DragDropManagerImpl = /*#__PURE__*/function () {
  function DragDropManagerImpl(store, monitor) {
    var _this = this;

    DragDropManagerImpl_classCallCheck(this, DragDropManagerImpl);

    DragDropManagerImpl_defineProperty(this, "store", void 0);

    DragDropManagerImpl_defineProperty(this, "monitor", void 0);

    DragDropManagerImpl_defineProperty(this, "backend", void 0);

    DragDropManagerImpl_defineProperty(this, "isSetUp", false);

    DragDropManagerImpl_defineProperty(this, "handleRefCountChange", function () {
      var shouldSetUp = _this.store.getState().refCount > 0;

      if (_this.backend) {
        if (shouldSetUp && !_this.isSetUp) {
          _this.backend.setup();

          _this.isSetUp = true;
        } else if (!shouldSetUp && _this.isSetUp) {
          _this.backend.teardown();

          _this.isSetUp = false;
        }
      }
    });

    this.store = store;
    this.monitor = monitor;
    store.subscribe(this.handleRefCountChange);
  }

  DragDropManagerImpl_createClass(DragDropManagerImpl, [{
    key: "receiveBackend",
    value: function receiveBackend(backend) {
      this.backend = backend;
    }
  }, {
    key: "getMonitor",
    value: function getMonitor() {
      return this.monitor;
    }
  }, {
    key: "getBackend",
    value: function getBackend() {
      return this.backend;
    }
  }, {
    key: "getRegistry",
    value: function getRegistry() {
      return this.monitor.registry;
    }
  }, {
    key: "getActions",
    value: function getActions() {
      /* eslint-disable-next-line @typescript-eslint/no-this-alias */
      var manager = this;
      var dispatch = this.store.dispatch;

      function bindActionCreator(actionCreator) {
        return function () {
          for (var _len = arguments.length, args = new Array(_len), _key = 0; _key < _len; _key++) {
            args[_key] = arguments[_key];
          }

          var action = actionCreator.apply(manager, args);

          if (typeof action !== 'undefined') {
            dispatch(action);
          }
        };
      }

      var actions = createDragDropActions(this);
      return Object.keys(actions).reduce(function (boundActions, key) {
        var action = actions[key];
        boundActions[key] = bindActionCreator(action);
        return boundActions;
      }, {});
    }
  }, {
    key: "dispatch",
    value: function dispatch(action) {
      this.store.dispatch(action);
    }
  }]);

  return DragDropManagerImpl;
}();
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/redux@4.2.1/node_modules/redux/es/redux.js


/**
 * Adapted from React: https://github.com/facebook/react/blob/master/packages/shared/formatProdErrorMessage.js
 *
 * Do not require this module directly! Use normal throw error calls. These messages will be replaced with error codes
 * during build.
 * @param {number} code
 */
function formatProdErrorMessage(code) {
  return "Minified Redux error #" + code + "; visit https://redux.js.org/Errors?code=" + code + " for the full message or " + 'use the non-minified dev environment for full errors. ';
}

// Inlined version of the `symbol-observable` polyfill
var $$observable = (function () {
  return typeof Symbol === 'function' && Symbol.observable || '@@observable';
})();

/**
 * These are private action types reserved by Redux.
 * For any unknown actions, you must return the current state.
 * If the current state is undefined, you must return the initial state.
 * Do not reference these action types directly in your code.
 */
var randomString = function randomString() {
  return Math.random().toString(36).substring(7).split('').join('.');
};

var ActionTypes = {
  INIT: "@@redux/INIT" + randomString(),
  REPLACE: "@@redux/REPLACE" + randomString(),
  PROBE_UNKNOWN_ACTION: function PROBE_UNKNOWN_ACTION() {
    return "@@redux/PROBE_UNKNOWN_ACTION" + randomString();
  }
};

/**
 * @param {any} obj The object to inspect.
 * @returns {boolean} True if the argument appears to be a plain object.
 */
function isPlainObject(obj) {
  if (typeof obj !== 'object' || obj === null) return false;
  var proto = obj;

  while (Object.getPrototypeOf(proto) !== null) {
    proto = Object.getPrototypeOf(proto);
  }

  return Object.getPrototypeOf(obj) === proto;
}

// Inlined / shortened version of `kindOf` from https://github.com/jonschlinkert/kind-of
function miniKindOf(val) {
  if (val === void 0) return 'undefined';
  if (val === null) return 'null';
  var type = typeof val;

  switch (type) {
    case 'boolean':
    case 'string':
    case 'number':
    case 'symbol':
    case 'function':
      {
        return type;
      }
  }

  if (Array.isArray(val)) return 'array';
  if (isDate(val)) return 'date';
  if (isError(val)) return 'error';
  var constructorName = ctorName(val);

  switch (constructorName) {
    case 'Symbol':
    case 'Promise':
    case 'WeakMap':
    case 'WeakSet':
    case 'Map':
    case 'Set':
      return constructorName;
  } // other


  return type.slice(8, -1).toLowerCase().replace(/\s/g, '');
}

function ctorName(val) {
  return typeof val.constructor === 'function' ? val.constructor.name : null;
}

function isError(val) {
  return val instanceof Error || typeof val.message === 'string' && val.constructor && typeof val.constructor.stackTraceLimit === 'number';
}

function isDate(val) {
  if (val instanceof Date) return true;
  return typeof val.toDateString === 'function' && typeof val.getDate === 'function' && typeof val.setDate === 'function';
}

function kindOf(val) {
  var typeOfVal = typeof val;

  if (false) {}

  return typeOfVal;
}

/**
 * @deprecated
 *
 * **We recommend using the `configureStore` method
 * of the `@reduxjs/toolkit` package**, which replaces `createStore`.
 *
 * Redux Toolkit is our recommended approach for writing Redux logic today,
 * including store setup, reducers, data fetching, and more.
 *
 * **For more details, please read this Redux docs page:**
 * **https://redux.js.org/introduction/why-rtk-is-redux-today**
 *
 * `configureStore` from Redux Toolkit is an improved version of `createStore` that
 * simplifies setup and helps avoid common bugs.
 *
 * You should not be using the `redux` core package by itself today, except for learning purposes.
 * The `createStore` method from the core `redux` package will not be removed, but we encourage
 * all users to migrate to using Redux Toolkit for all Redux code.
 *
 * If you want to use `createStore` without this visual deprecation warning, use
 * the `legacy_createStore` import instead:
 *
 * `import { legacy_createStore as createStore} from 'redux'`
 *
 */

function createStore(reducer, preloadedState, enhancer) {
  var _ref2;

  if (typeof preloadedState === 'function' && typeof enhancer === 'function' || typeof enhancer === 'function' && typeof arguments[3] === 'function') {
    throw new Error( true ? formatProdErrorMessage(0) : 0);
  }

  if (typeof preloadedState === 'function' && typeof enhancer === 'undefined') {
    enhancer = preloadedState;
    preloadedState = undefined;
  }

  if (typeof enhancer !== 'undefined') {
    if (typeof enhancer !== 'function') {
      throw new Error( true ? formatProdErrorMessage(1) : 0);
    }

    return enhancer(createStore)(reducer, preloadedState);
  }

  if (typeof reducer !== 'function') {
    throw new Error( true ? formatProdErrorMessage(2) : 0);
  }

  var currentReducer = reducer;
  var currentState = preloadedState;
  var currentListeners = [];
  var nextListeners = currentListeners;
  var isDispatching = false;
  /**
   * This makes a shallow copy of currentListeners so we can use
   * nextListeners as a temporary list while dispatching.
   *
   * This prevents any bugs around consumers calling
   * subscribe/unsubscribe in the middle of a dispatch.
   */

  function ensureCanMutateNextListeners() {
    if (nextListeners === currentListeners) {
      nextListeners = currentListeners.slice();
    }
  }
  /**
   * Reads the state tree managed by the store.
   *
   * @returns {any} The current state tree of your application.
   */


  function getState() {
    if (isDispatching) {
      throw new Error( true ? formatProdErrorMessage(3) : 0);
    }

    return currentState;
  }
  /**
   * Adds a change listener. It will be called any time an action is dispatched,
   * and some part of the state tree may potentially have changed. You may then
   * call `getState()` to read the current state tree inside the callback.
   *
   * You may call `dispatch()` from a change listener, with the following
   * caveats:
   *
   * 1. The subscriptions are snapshotted just before every `dispatch()` call.
   * If you subscribe or unsubscribe while the listeners are being invoked, this
   * will not have any effect on the `dispatch()` that is currently in progress.
   * However, the next `dispatch()` call, whether nested or not, will use a more
   * recent snapshot of the subscription list.
   *
   * 2. The listener should not expect to see all state changes, as the state
   * might have been updated multiple times during a nested `dispatch()` before
   * the listener is called. It is, however, guaranteed that all subscribers
   * registered before the `dispatch()` started will be called with the latest
   * state by the time it exits.
   *
   * @param {Function} listener A callback to be invoked on every dispatch.
   * @returns {Function} A function to remove this change listener.
   */


  function subscribe(listener) {
    if (typeof listener !== 'function') {
      throw new Error( true ? formatProdErrorMessage(4) : 0);
    }

    if (isDispatching) {
      throw new Error( true ? formatProdErrorMessage(5) : 0);
    }

    var isSubscribed = true;
    ensureCanMutateNextListeners();
    nextListeners.push(listener);
    return function unsubscribe() {
      if (!isSubscribed) {
        return;
      }

      if (isDispatching) {
        throw new Error( true ? formatProdErrorMessage(6) : 0);
      }

      isSubscribed = false;
      ensureCanMutateNextListeners();
      var index = nextListeners.indexOf(listener);
      nextListeners.splice(index, 1);
      currentListeners = null;
    };
  }
  /**
   * Dispatches an action. It is the only way to trigger a state change.
   *
   * The `reducer` function, used to create the store, will be called with the
   * current state tree and the given `action`. Its return value will
   * be considered the **next** state of the tree, and the change listeners
   * will be notified.
   *
   * The base implementation only supports plain object actions. If you want to
   * dispatch a Promise, an Observable, a thunk, or something else, you need to
   * wrap your store creating function into the corresponding middleware. For
   * example, see the documentation for the `redux-thunk` package. Even the
   * middleware will eventually dispatch plain object actions using this method.
   *
   * @param {Object} action A plain object representing “what changed”. It is
   * a good idea to keep actions serializable so you can record and replay user
   * sessions, or use the time travelling `redux-devtools`. An action must have
   * a `type` property which may not be `undefined`. It is a good idea to use
   * string constants for action types.
   *
   * @returns {Object} For convenience, the same action object you dispatched.
   *
   * Note that, if you use a custom middleware, it may wrap `dispatch()` to
   * return something else (for example, a Promise you can await).
   */


  function dispatch(action) {
    if (!isPlainObject(action)) {
      throw new Error( true ? formatProdErrorMessage(7) : 0);
    }

    if (typeof action.type === 'undefined') {
      throw new Error( true ? formatProdErrorMessage(8) : 0);
    }

    if (isDispatching) {
      throw new Error( true ? formatProdErrorMessage(9) : 0);
    }

    try {
      isDispatching = true;
      currentState = currentReducer(currentState, action);
    } finally {
      isDispatching = false;
    }

    var listeners = currentListeners = nextListeners;

    for (var i = 0; i < listeners.length; i++) {
      var listener = listeners[i];
      listener();
    }

    return action;
  }
  /**
   * Replaces the reducer currently used by the store to calculate the state.
   *
   * You might need this if your app implements code splitting and you want to
   * load some of the reducers dynamically. You might also need this if you
   * implement a hot reloading mechanism for Redux.
   *
   * @param {Function} nextReducer The reducer for the store to use instead.
   * @returns {void}
   */


  function replaceReducer(nextReducer) {
    if (typeof nextReducer !== 'function') {
      throw new Error( true ? formatProdErrorMessage(10) : 0);
    }

    currentReducer = nextReducer; // This action has a similiar effect to ActionTypes.INIT.
    // Any reducers that existed in both the new and old rootReducer
    // will receive the previous state. This effectively populates
    // the new state tree with any relevant data from the old one.

    dispatch({
      type: ActionTypes.REPLACE
    });
  }
  /**
   * Interoperability point for observable/reactive libraries.
   * @returns {observable} A minimal observable of state changes.
   * For more information, see the observable proposal:
   * https://github.com/tc39/proposal-observable
   */


  function observable() {
    var _ref;

    var outerSubscribe = subscribe;
    return _ref = {
      /**
       * The minimal observable subscription method.
       * @param {Object} observer Any object that can be used as an observer.
       * The observer object should have a `next` method.
       * @returns {subscription} An object with an `unsubscribe` method that can
       * be used to unsubscribe the observable from the store, and prevent further
       * emission of values from the observable.
       */
      subscribe: function subscribe(observer) {
        if (typeof observer !== 'object' || observer === null) {
          throw new Error( true ? formatProdErrorMessage(11) : 0);
        }

        function observeState() {
          if (observer.next) {
            observer.next(getState());
          }
        }

        observeState();
        var unsubscribe = outerSubscribe(observeState);
        return {
          unsubscribe: unsubscribe
        };
      }
    }, _ref[$$observable] = function () {
      return this;
    }, _ref;
  } // When a store is created, an "INIT" action is dispatched so that every
  // reducer returns their initial state. This effectively populates
  // the initial state tree.


  dispatch({
    type: ActionTypes.INIT
  });
  return _ref2 = {
    dispatch: dispatch,
    subscribe: subscribe,
    getState: getState,
    replaceReducer: replaceReducer
  }, _ref2[$$observable] = observable, _ref2;
}
/**
 * Creates a Redux store that holds the state tree.
 *
 * **We recommend using `configureStore` from the
 * `@reduxjs/toolkit` package**, which replaces `createStore`:
 * **https://redux.js.org/introduction/why-rtk-is-redux-today**
 *
 * The only way to change the data in the store is to call `dispatch()` on it.
 *
 * There should only be a single store in your app. To specify how different
 * parts of the state tree respond to actions, you may combine several reducers
 * into a single reducer function by using `combineReducers`.
 *
 * @param {Function} reducer A function that returns the next state tree, given
 * the current state tree and the action to handle.
 *
 * @param {any} [preloadedState] The initial state. You may optionally specify it
 * to hydrate the state from the server in universal apps, or to restore a
 * previously serialized user session.
 * If you use `combineReducers` to produce the root reducer function, this must be
 * an object with the same shape as `combineReducers` keys.
 *
 * @param {Function} [enhancer] The store enhancer. You may optionally specify it
 * to enhance the store with third-party capabilities such as middleware,
 * time travel, persistence, etc. The only store enhancer that ships with Redux
 * is `applyMiddleware()`.
 *
 * @returns {Store} A Redux store that lets you read the state, dispatch actions
 * and subscribe to changes.
 */

var legacy_createStore = (/* unused pure expression or super */ null && (createStore));

/**
 * Prints a warning in the console if it exists.
 *
 * @param {String} message The warning message.
 * @returns {void}
 */
function warning(message) {
  /* eslint-disable no-console */
  if (typeof console !== 'undefined' && typeof console.error === 'function') {
    console.error(message);
  }
  /* eslint-enable no-console */


  try {
    // This error was thrown as a convenience so that if you enable
    // "break on all exceptions" in your console,
    // it would pause the execution at this line.
    throw new Error(message);
  } catch (e) {} // eslint-disable-line no-empty

}

function getUnexpectedStateShapeWarningMessage(inputState, reducers, action, unexpectedKeyCache) {
  var reducerKeys = Object.keys(reducers);
  var argumentName = action && action.type === ActionTypes.INIT ? 'preloadedState argument passed to createStore' : 'previous state received by the reducer';

  if (reducerKeys.length === 0) {
    return 'Store does not have a valid reducer. Make sure the argument passed ' + 'to combineReducers is an object whose values are reducers.';
  }

  if (!isPlainObject(inputState)) {
    return "The " + argumentName + " has unexpected type of \"" + kindOf(inputState) + "\". Expected argument to be an object with the following " + ("keys: \"" + reducerKeys.join('", "') + "\"");
  }

  var unexpectedKeys = Object.keys(inputState).filter(function (key) {
    return !reducers.hasOwnProperty(key) && !unexpectedKeyCache[key];
  });
  unexpectedKeys.forEach(function (key) {
    unexpectedKeyCache[key] = true;
  });
  if (action && action.type === ActionTypes.REPLACE) return;

  if (unexpectedKeys.length > 0) {
    return "Unexpected " + (unexpectedKeys.length > 1 ? 'keys' : 'key') + " " + ("\"" + unexpectedKeys.join('", "') + "\" found in " + argumentName + ". ") + "Expected to find one of the known reducer keys instead: " + ("\"" + reducerKeys.join('", "') + "\". Unexpected keys will be ignored.");
  }
}

function assertReducerShape(reducers) {
  Object.keys(reducers).forEach(function (key) {
    var reducer = reducers[key];
    var initialState = reducer(undefined, {
      type: ActionTypes.INIT
    });

    if (typeof initialState === 'undefined') {
      throw new Error( true ? formatProdErrorMessage(12) : 0);
    }

    if (typeof reducer(undefined, {
      type: ActionTypes.PROBE_UNKNOWN_ACTION()
    }) === 'undefined') {
      throw new Error( true ? formatProdErrorMessage(13) : 0);
    }
  });
}
/**
 * Turns an object whose values are different reducer functions, into a single
 * reducer function. It will call every child reducer, and gather their results
 * into a single state object, whose keys correspond to the keys of the passed
 * reducer functions.
 *
 * @param {Object} reducers An object whose values correspond to different
 * reducer functions that need to be combined into one. One handy way to obtain
 * it is to use ES6 `import * as reducers` syntax. The reducers may never return
 * undefined for any action. Instead, they should return their initial state
 * if the state passed to them was undefined, and the current state for any
 * unrecognized action.
 *
 * @returns {Function} A reducer function that invokes every reducer inside the
 * passed object, and builds a state object with the same shape.
 */


function combineReducers(reducers) {
  var reducerKeys = Object.keys(reducers);
  var finalReducers = {};

  for (var i = 0; i < reducerKeys.length; i++) {
    var key = reducerKeys[i];

    if (false) {}

    if (typeof reducers[key] === 'function') {
      finalReducers[key] = reducers[key];
    }
  }

  var finalReducerKeys = Object.keys(finalReducers); // This is used to make sure we don't warn about the same
  // keys multiple times.

  var unexpectedKeyCache;

  if (false) {}

  var shapeAssertionError;

  try {
    assertReducerShape(finalReducers);
  } catch (e) {
    shapeAssertionError = e;
  }

  return function combination(state, action) {
    if (state === void 0) {
      state = {};
    }

    if (shapeAssertionError) {
      throw shapeAssertionError;
    }

    if (false) { var warningMessage; }

    var hasChanged = false;
    var nextState = {};

    for (var _i = 0; _i < finalReducerKeys.length; _i++) {
      var _key = finalReducerKeys[_i];
      var reducer = finalReducers[_key];
      var previousStateForKey = state[_key];
      var nextStateForKey = reducer(previousStateForKey, action);

      if (typeof nextStateForKey === 'undefined') {
        var actionType = action && action.type;
        throw new Error( true ? formatProdErrorMessage(14) : 0);
      }

      nextState[_key] = nextStateForKey;
      hasChanged = hasChanged || nextStateForKey !== previousStateForKey;
    }

    hasChanged = hasChanged || finalReducerKeys.length !== Object.keys(state).length;
    return hasChanged ? nextState : state;
  };
}

function bindActionCreator(actionCreator, dispatch) {
  return function () {
    return dispatch(actionCreator.apply(this, arguments));
  };
}
/**
 * Turns an object whose values are action creators, into an object with the
 * same keys, but with every function wrapped into a `dispatch` call so they
 * may be invoked directly. This is just a convenience method, as you can call
 * `store.dispatch(MyActionCreators.doSomething())` yourself just fine.
 *
 * For convenience, you can also pass an action creator as the first argument,
 * and get a dispatch wrapped function in return.
 *
 * @param {Function|Object} actionCreators An object whose values are action
 * creator functions. One handy way to obtain it is to use ES6 `import * as`
 * syntax. You may also pass a single function.
 *
 * @param {Function} dispatch The `dispatch` function available on your Redux
 * store.
 *
 * @returns {Function|Object} The object mimicking the original object, but with
 * every action creator wrapped into the `dispatch` call. If you passed a
 * function as `actionCreators`, the return value will also be a single
 * function.
 */


function bindActionCreators(actionCreators, dispatch) {
  if (typeof actionCreators === 'function') {
    return bindActionCreator(actionCreators, dispatch);
  }

  if (typeof actionCreators !== 'object' || actionCreators === null) {
    throw new Error( true ? formatProdErrorMessage(16) : 0);
  }

  var boundActionCreators = {};

  for (var key in actionCreators) {
    var actionCreator = actionCreators[key];

    if (typeof actionCreator === 'function') {
      boundActionCreators[key] = bindActionCreator(actionCreator, dispatch);
    }
  }

  return boundActionCreators;
}

/**
 * Composes single-argument functions from right to left. The rightmost
 * function can take multiple arguments as it provides the signature for
 * the resulting composite function.
 *
 * @param {...Function} funcs The functions to compose.
 * @returns {Function} A function obtained by composing the argument functions
 * from right to left. For example, compose(f, g, h) is identical to doing
 * (...args) => f(g(h(...args))).
 */
function compose() {
  for (var _len = arguments.length, funcs = new Array(_len), _key = 0; _key < _len; _key++) {
    funcs[_key] = arguments[_key];
  }

  if (funcs.length === 0) {
    return function (arg) {
      return arg;
    };
  }

  if (funcs.length === 1) {
    return funcs[0];
  }

  return funcs.reduce(function (a, b) {
    return function () {
      return a(b.apply(void 0, arguments));
    };
  });
}

/**
 * Creates a store enhancer that applies middleware to the dispatch method
 * of the Redux store. This is handy for a variety of tasks, such as expressing
 * asynchronous actions in a concise manner, or logging every action payload.
 *
 * See `redux-thunk` package as an example of the Redux middleware.
 *
 * Because middleware is potentially asynchronous, this should be the first
 * store enhancer in the composition chain.
 *
 * Note that each middleware will be given the `dispatch` and `getState` functions
 * as named arguments.
 *
 * @param {...Function} middlewares The middleware chain to be applied.
 * @returns {Function} A store enhancer applying the middleware.
 */

function applyMiddleware() {
  for (var _len = arguments.length, middlewares = new Array(_len), _key = 0; _key < _len; _key++) {
    middlewares[_key] = arguments[_key];
  }

  return function (createStore) {
    return function () {
      var store = createStore.apply(void 0, arguments);

      var _dispatch = function dispatch() {
        throw new Error( true ? formatProdErrorMessage(15) : 0);
      };

      var middlewareAPI = {
        getState: store.getState,
        dispatch: function dispatch() {
          return _dispatch.apply(void 0, arguments);
        }
      };
      var chain = middlewares.map(function (middleware) {
        return middleware(middlewareAPI);
      });
      _dispatch = compose.apply(void 0, chain)(store.dispatch);
      return _objectSpread(_objectSpread({}, store), {}, {
        dispatch: _dispatch
      });
    };
  };
}



;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/utils/equality.js
var strictEquality = function strictEquality(a, b) {
  return a === b;
};
/**
 * Determine if two cartesian coordinate offsets are equal
 * @param offsetA
 * @param offsetB
 */

function areCoordsEqual(offsetA, offsetB) {
  if (!offsetA && !offsetB) {
    return true;
  } else if (!offsetA || !offsetB) {
    return false;
  } else {
    return offsetA.x === offsetB.x && offsetA.y === offsetB.y;
  }
}
/**
 * Determines if two arrays of items are equal
 * @param a The first array of items
 * @param b The second array of items
 */

function areArraysEqual(a, b) {
  var isEqual = arguments.length > 2 && arguments[2] !== undefined ? arguments[2] : strictEquality;

  if (a.length !== b.length) {
    return false;
  }

  for (var i = 0; i < a.length; ++i) {
    if (!isEqual(a[i], b[i])) {
      return false;
    }
  }

  return true;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/reducers/dragOffset.js
function dragOffset_ownKeys(object, enumerableOnly) { var keys = Object.keys(object); if (Object.getOwnPropertySymbols) { var symbols = Object.getOwnPropertySymbols(object); if (enumerableOnly) { symbols = symbols.filter(function (sym) { return Object.getOwnPropertyDescriptor(object, sym).enumerable; }); } keys.push.apply(keys, symbols); } return keys; }

function dragOffset_objectSpread(target) { for (var i = 1; i < arguments.length; i++) { var source = arguments[i] != null ? arguments[i] : {}; if (i % 2) { dragOffset_ownKeys(Object(source), true).forEach(function (key) { dragOffset_defineProperty(target, key, source[key]); }); } else if (Object.getOwnPropertyDescriptors) { Object.defineProperties(target, Object.getOwnPropertyDescriptors(source)); } else { dragOffset_ownKeys(Object(source)).forEach(function (key) { Object.defineProperty(target, key, Object.getOwnPropertyDescriptor(source, key)); }); } } return target; }

function dragOffset_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }



var dragOffset_initialState = {
  initialSourceClientOffset: null,
  initialClientOffset: null,
  clientOffset: null
};
function reduce() {
  var state = arguments.length > 0 && arguments[0] !== undefined ? arguments[0] : dragOffset_initialState;
  var action = arguments.length > 1 ? arguments[1] : undefined;
  var payload = action.payload;

  switch (action.type) {
    case INIT_COORDS:
    case BEGIN_DRAG:
      return {
        initialSourceClientOffset: payload.sourceClientOffset,
        initialClientOffset: payload.clientOffset,
        clientOffset: payload.clientOffset
      };

    case HOVER:
      if (areCoordsEqual(state.clientOffset, payload.clientOffset)) {
        return state;
      }

      return dragOffset_objectSpread(dragOffset_objectSpread({}, state), {}, {
        clientOffset: payload.clientOffset
      });

    case END_DRAG:
    case DROP:
      return dragOffset_initialState;

    default:
      return state;
  }
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/actions/registry.js
var ADD_SOURCE = 'dnd-core/ADD_SOURCE';
var ADD_TARGET = 'dnd-core/ADD_TARGET';
var REMOVE_SOURCE = 'dnd-core/REMOVE_SOURCE';
var REMOVE_TARGET = 'dnd-core/REMOVE_TARGET';
function registry_addSource(sourceId) {
  return {
    type: ADD_SOURCE,
    payload: {
      sourceId: sourceId
    }
  };
}
function registry_addTarget(targetId) {
  return {
    type: ADD_TARGET,
    payload: {
      targetId: targetId
    }
  };
}
function registry_removeSource(sourceId) {
  return {
    type: REMOVE_SOURCE,
    payload: {
      sourceId: sourceId
    }
  };
}
function registry_removeTarget(targetId) {
  return {
    type: REMOVE_TARGET,
    payload: {
      targetId: targetId
    }
  };
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/reducers/dragOperation.js
function dragOperation_ownKeys(object, enumerableOnly) { var keys = Object.keys(object); if (Object.getOwnPropertySymbols) { var symbols = Object.getOwnPropertySymbols(object); if (enumerableOnly) { symbols = symbols.filter(function (sym) { return Object.getOwnPropertyDescriptor(object, sym).enumerable; }); } keys.push.apply(keys, symbols); } return keys; }

function dragOperation_objectSpread(target) { for (var i = 1; i < arguments.length; i++) { var source = arguments[i] != null ? arguments[i] : {}; if (i % 2) { dragOperation_ownKeys(Object(source), true).forEach(function (key) { dragOperation_defineProperty(target, key, source[key]); }); } else if (Object.getOwnPropertyDescriptors) { Object.defineProperties(target, Object.getOwnPropertyDescriptors(source)); } else { dragOperation_ownKeys(Object(source)).forEach(function (key) { Object.defineProperty(target, key, Object.getOwnPropertyDescriptor(source, key)); }); } } return target; }

function dragOperation_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }




var dragOperation_initialState = {
  itemType: null,
  item: null,
  sourceId: null,
  targetIds: [],
  dropResult: null,
  didDrop: false,
  isSourcePublic: null
};
function dragOperation_reduce() {
  var state = arguments.length > 0 && arguments[0] !== undefined ? arguments[0] : dragOperation_initialState;
  var action = arguments.length > 1 ? arguments[1] : undefined;
  var payload = action.payload;

  switch (action.type) {
    case BEGIN_DRAG:
      return dragOperation_objectSpread(dragOperation_objectSpread({}, state), {}, {
        itemType: payload.itemType,
        item: payload.item,
        sourceId: payload.sourceId,
        isSourcePublic: payload.isSourcePublic,
        dropResult: null,
        didDrop: false
      });

    case PUBLISH_DRAG_SOURCE:
      return dragOperation_objectSpread(dragOperation_objectSpread({}, state), {}, {
        isSourcePublic: true
      });

    case HOVER:
      return dragOperation_objectSpread(dragOperation_objectSpread({}, state), {}, {
        targetIds: payload.targetIds
      });

    case REMOVE_TARGET:
      if (state.targetIds.indexOf(payload.targetId) === -1) {
        return state;
      }

      return dragOperation_objectSpread(dragOperation_objectSpread({}, state), {}, {
        targetIds: js_utils_without(state.targetIds, payload.targetId)
      });

    case DROP:
      return dragOperation_objectSpread(dragOperation_objectSpread({}, state), {}, {
        dropResult: payload.dropResult,
        didDrop: true,
        targetIds: []
      });

    case END_DRAG:
      return dragOperation_objectSpread(dragOperation_objectSpread({}, state), {}, {
        itemType: null,
        item: null,
        sourceId: null,
        dropResult: null,
        didDrop: false,
        isSourcePublic: null,
        targetIds: []
      });

    default:
      return state;
  }
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/reducers/refCount.js

function refCount_reduce() {
  var state = arguments.length > 0 && arguments[0] !== undefined ? arguments[0] : 0;
  var action = arguments.length > 1 ? arguments[1] : undefined;

  switch (action.type) {
    case ADD_SOURCE:
    case ADD_TARGET:
      return state + 1;

    case REMOVE_SOURCE:
    case REMOVE_TARGET:
      return state - 1;

    default:
      return state;
  }
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/utils/dirtiness.js

var NONE = [];
var ALL = [];
NONE.__IS_NONE__ = true;
ALL.__IS_ALL__ = true;
/**
 * Determines if the given handler IDs are dirty or not.
 *
 * @param dirtyIds The set of dirty handler ids
 * @param handlerIds The set of handler ids to check
 */

function areDirty(dirtyIds, handlerIds) {
  if (dirtyIds === NONE) {
    return false;
  }

  if (dirtyIds === ALL || typeof handlerIds === 'undefined') {
    return true;
  }

  var commonIds = intersection(handlerIds, dirtyIds);
  return commonIds.length > 0;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/reducers/dirtyHandlerIds.js





function dirtyHandlerIds_reduce() {
  var _state = arguments.length > 0 && arguments[0] !== undefined ? arguments[0] : NONE;

  var action = arguments.length > 1 ? arguments[1] : undefined;

  switch (action.type) {
    case HOVER:
      break;

    case ADD_SOURCE:
    case ADD_TARGET:
    case REMOVE_TARGET:
    case REMOVE_SOURCE:
      return NONE;

    case BEGIN_DRAG:
    case PUBLISH_DRAG_SOURCE:
    case END_DRAG:
    case DROP:
    default:
      return ALL;
  }

  var _action$payload = action.payload,
      _action$payload$targe = _action$payload.targetIds,
      targetIds = _action$payload$targe === void 0 ? [] : _action$payload$targe,
      _action$payload$prevT = _action$payload.prevTargetIds,
      prevTargetIds = _action$payload$prevT === void 0 ? [] : _action$payload$prevT;
  var result = xor(targetIds, prevTargetIds);
  var didChange = result.length > 0 || !areArraysEqual(targetIds, prevTargetIds);

  if (!didChange) {
    return NONE;
  } // Check the target ids at the innermost position. If they are valid, add them
  // to the result


  var prevInnermostTargetId = prevTargetIds[prevTargetIds.length - 1];
  var innermostTargetId = targetIds[targetIds.length - 1];

  if (prevInnermostTargetId !== innermostTargetId) {
    if (prevInnermostTargetId) {
      result.push(prevInnermostTargetId);
    }

    if (innermostTargetId) {
      result.push(innermostTargetId);
    }
  }

  return result;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/reducers/stateId.js
function stateId_reduce() {
  var state = arguments.length > 0 && arguments[0] !== undefined ? arguments[0] : 0;
  return state + 1;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/reducers/index.js
function reducers_ownKeys(object, enumerableOnly) { var keys = Object.keys(object); if (Object.getOwnPropertySymbols) { var symbols = Object.getOwnPropertySymbols(object); if (enumerableOnly) { symbols = symbols.filter(function (sym) { return Object.getOwnPropertyDescriptor(object, sym).enumerable; }); } keys.push.apply(keys, symbols); } return keys; }

function reducers_objectSpread(target) { for (var i = 1; i < arguments.length; i++) { var source = arguments[i] != null ? arguments[i] : {}; if (i % 2) { reducers_ownKeys(Object(source), true).forEach(function (key) { reducers_defineProperty(target, key, source[key]); }); } else if (Object.getOwnPropertyDescriptors) { Object.defineProperties(target, Object.getOwnPropertyDescriptors(source)); } else { reducers_ownKeys(Object(source)).forEach(function (key) { Object.defineProperty(target, key, Object.getOwnPropertyDescriptor(source, key)); }); } } return target; }

function reducers_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }







function reducers_reduce() {
  var state = arguments.length > 0 && arguments[0] !== undefined ? arguments[0] : {};
  var action = arguments.length > 1 ? arguments[1] : undefined;
  return {
    dirtyHandlerIds: dirtyHandlerIds_reduce(state.dirtyHandlerIds, {
      type: action.type,
      payload: reducers_objectSpread(reducers_objectSpread({}, action.payload), {}, {
        prevTargetIds: get(state, 'dragOperation.targetIds', [])
      })
    }),
    dragOffset: reduce(state.dragOffset, action),
    refCount: refCount_reduce(state.refCount, action),
    dragOperation: dragOperation_reduce(state.dragOperation, action),
    stateId: stateId_reduce(state.stateId)
  };
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/utils/coords.js
/**
 * Coordinate addition
 * @param a The first coordinate
 * @param b The second coordinate
 */
function add(a, b) {
  return {
    x: a.x + b.x,
    y: a.y + b.y
  };
}
/**
 * Coordinate subtraction
 * @param a The first coordinate
 * @param b The second coordinate
 */

function subtract(a, b) {
  return {
    x: a.x - b.x,
    y: a.y - b.y
  };
}
/**
 * Returns the cartesian distance of the drag source component's position, based on its position
 * at the time when the current drag operation has started, and the movement difference.
 *
 * Returns null if no item is being dragged.
 *
 * @param state The offset state to compute from
 */

function coords_getSourceClientOffset(state) {
  var clientOffset = state.clientOffset,
      initialClientOffset = state.initialClientOffset,
      initialSourceClientOffset = state.initialSourceClientOffset;

  if (!clientOffset || !initialClientOffset || !initialSourceClientOffset) {
    return null;
  }

  return subtract(add(clientOffset, initialSourceClientOffset), initialClientOffset);
}
/**
 * Determines the x,y offset between the client offset and the initial client offset
 *
 * @param state The offset state to compute from
 */

function coords_getDifferenceFromInitialOffset(state) {
  var clientOffset = state.clientOffset,
      initialClientOffset = state.initialClientOffset;

  if (!clientOffset || !initialClientOffset) {
    return null;
  }

  return subtract(clientOffset, initialClientOffset);
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/classes/DragDropMonitorImpl.js
function DragDropMonitorImpl_classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function DragDropMonitorImpl_defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function DragDropMonitorImpl_createClass(Constructor, protoProps, staticProps) { if (protoProps) DragDropMonitorImpl_defineProperties(Constructor.prototype, protoProps); if (staticProps) DragDropMonitorImpl_defineProperties(Constructor, staticProps); return Constructor; }

function DragDropMonitorImpl_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }





var DragDropMonitorImpl = /*#__PURE__*/function () {
  function DragDropMonitorImpl(store, registry) {
    DragDropMonitorImpl_classCallCheck(this, DragDropMonitorImpl);

    DragDropMonitorImpl_defineProperty(this, "store", void 0);

    DragDropMonitorImpl_defineProperty(this, "registry", void 0);

    this.store = store;
    this.registry = registry;
  }

  DragDropMonitorImpl_createClass(DragDropMonitorImpl, [{
    key: "subscribeToStateChange",
    value: function subscribeToStateChange(listener) {
      var _this = this;

      var options = arguments.length > 1 && arguments[1] !== undefined ? arguments[1] : {
        handlerIds: undefined
      };
      var handlerIds = options.handlerIds;
      invariant(typeof listener === 'function', 'listener must be a function.');
      invariant(typeof handlerIds === 'undefined' || Array.isArray(handlerIds), 'handlerIds, when specified, must be an array of strings.');
      var prevStateId = this.store.getState().stateId;

      var handleChange = function handleChange() {
        var state = _this.store.getState();

        var currentStateId = state.stateId;

        try {
          var canSkipListener = currentStateId === prevStateId || currentStateId === prevStateId + 1 && !areDirty(state.dirtyHandlerIds, handlerIds);

          if (!canSkipListener) {
            listener();
          }
        } finally {
          prevStateId = currentStateId;
        }
      };

      return this.store.subscribe(handleChange);
    }
  }, {
    key: "subscribeToOffsetChange",
    value: function subscribeToOffsetChange(listener) {
      var _this2 = this;

      invariant(typeof listener === 'function', 'listener must be a function.');
      var previousState = this.store.getState().dragOffset;

      var handleChange = function handleChange() {
        var nextState = _this2.store.getState().dragOffset;

        if (nextState === previousState) {
          return;
        }

        previousState = nextState;
        listener();
      };

      return this.store.subscribe(handleChange);
    }
  }, {
    key: "canDragSource",
    value: function canDragSource(sourceId) {
      if (!sourceId) {
        return false;
      }

      var source = this.registry.getSource(sourceId);
      invariant(source, "Expected to find a valid source. sourceId=".concat(sourceId));

      if (this.isDragging()) {
        return false;
      }

      return source.canDrag(this, sourceId);
    }
  }, {
    key: "canDropOnTarget",
    value: function canDropOnTarget(targetId) {
      // undefined on initial render
      if (!targetId) {
        return false;
      }

      var target = this.registry.getTarget(targetId);
      invariant(target, "Expected to find a valid target. targetId=".concat(targetId));

      if (!this.isDragging() || this.didDrop()) {
        return false;
      }

      var targetType = this.registry.getTargetType(targetId);
      var draggedItemType = this.getItemType();
      return matchesType(targetType, draggedItemType) && target.canDrop(this, targetId);
    }
  }, {
    key: "isDragging",
    value: function isDragging() {
      return Boolean(this.getItemType());
    }
  }, {
    key: "isDraggingSource",
    value: function isDraggingSource(sourceId) {
      // undefined on initial render
      if (!sourceId) {
        return false;
      }

      var source = this.registry.getSource(sourceId, true);
      invariant(source, "Expected to find a valid source. sourceId=".concat(sourceId));

      if (!this.isDragging() || !this.isSourcePublic()) {
        return false;
      }

      var sourceType = this.registry.getSourceType(sourceId);
      var draggedItemType = this.getItemType();

      if (sourceType !== draggedItemType) {
        return false;
      }

      return source.isDragging(this, sourceId);
    }
  }, {
    key: "isOverTarget",
    value: function isOverTarget(targetId) {
      var options = arguments.length > 1 && arguments[1] !== undefined ? arguments[1] : {
        shallow: false
      };

      // undefined on initial render
      if (!targetId) {
        return false;
      }

      var shallow = options.shallow;

      if (!this.isDragging()) {
        return false;
      }

      var targetType = this.registry.getTargetType(targetId);
      var draggedItemType = this.getItemType();

      if (draggedItemType && !matchesType(targetType, draggedItemType)) {
        return false;
      }

      var targetIds = this.getTargetIds();

      if (!targetIds.length) {
        return false;
      }

      var index = targetIds.indexOf(targetId);

      if (shallow) {
        return index === targetIds.length - 1;
      } else {
        return index > -1;
      }
    }
  }, {
    key: "getItemType",
    value: function getItemType() {
      return this.store.getState().dragOperation.itemType;
    }
  }, {
    key: "getItem",
    value: function getItem() {
      return this.store.getState().dragOperation.item;
    }
  }, {
    key: "getSourceId",
    value: function getSourceId() {
      return this.store.getState().dragOperation.sourceId;
    }
  }, {
    key: "getTargetIds",
    value: function getTargetIds() {
      return this.store.getState().dragOperation.targetIds;
    }
  }, {
    key: "getDropResult",
    value: function getDropResult() {
      return this.store.getState().dragOperation.dropResult;
    }
  }, {
    key: "didDrop",
    value: function didDrop() {
      return this.store.getState().dragOperation.didDrop;
    }
  }, {
    key: "isSourcePublic",
    value: function isSourcePublic() {
      return Boolean(this.store.getState().dragOperation.isSourcePublic);
    }
  }, {
    key: "getInitialClientOffset",
    value: function getInitialClientOffset() {
      return this.store.getState().dragOffset.initialClientOffset;
    }
  }, {
    key: "getInitialSourceClientOffset",
    value: function getInitialSourceClientOffset() {
      return this.store.getState().dragOffset.initialSourceClientOffset;
    }
  }, {
    key: "getClientOffset",
    value: function getClientOffset() {
      return this.store.getState().dragOffset.clientOffset;
    }
  }, {
    key: "getSourceClientOffset",
    value: function getSourceClientOffset() {
      return coords_getSourceClientOffset(this.store.getState().dragOffset);
    }
  }, {
    key: "getDifferenceFromInitialOffset",
    value: function getDifferenceFromInitialOffset() {
      return coords_getDifferenceFromInitialOffset(this.store.getState().dragOffset);
    }
  }]);

  return DragDropMonitorImpl;
}();
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/utils/getNextUniqueId.js
var nextUniqueId = 0;
function getNextUniqueId() {
  return nextUniqueId++;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/interfaces.js
var HandlerRole;

(function (HandlerRole) {
  HandlerRole["SOURCE"] = "SOURCE";
  HandlerRole["TARGET"] = "TARGET";
})(HandlerRole || (HandlerRole = {}));
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/contracts.js
function contracts_typeof(obj) { "@babel/helpers - typeof"; if (typeof Symbol === "function" && typeof Symbol.iterator === "symbol") { contracts_typeof = function _typeof(obj) { return typeof obj; }; } else { contracts_typeof = function _typeof(obj) { return obj && typeof Symbol === "function" && obj.constructor === Symbol && obj !== Symbol.prototype ? "symbol" : typeof obj; }; } return contracts_typeof(obj); }


function validateSourceContract(source) {
  invariant(typeof source.canDrag === 'function', 'Expected canDrag to be a function.');
  invariant(typeof source.beginDrag === 'function', 'Expected beginDrag to be a function.');
  invariant(typeof source.endDrag === 'function', 'Expected endDrag to be a function.');
}
function validateTargetContract(target) {
  invariant(typeof target.canDrop === 'function', 'Expected canDrop to be a function.');
  invariant(typeof target.hover === 'function', 'Expected hover to be a function.');
  invariant(typeof target.drop === 'function', 'Expected beginDrag to be a function.');
}
function validateType(type, allowArray) {
  if (allowArray && Array.isArray(type)) {
    type.forEach(function (t) {
      return validateType(t, false);
    });
    return;
  }

  invariant(typeof type === 'string' || contracts_typeof(type) === 'symbol', allowArray ? 'Type can only be a string, a symbol, or an array of either.' : 'Type can only be a string or a symbol.');
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@react-dnd+asap@4.0.1/node_modules/@react-dnd/asap/dist/esm/makeRequestCall.mjs
// Safari 6 and 6.1 for desktop, iPad, and iPhone are the only browsers that
// have WebKitMutationObserver but not un-prefixed MutationObserver.
// Must use `global` or `self` instead of `window` to work in both frames and web
// workers. `global` is a provision of Browserify, Mr, Mrs, or Mop.
/* globals self */ const scope = typeof global !== 'undefined' ? global : self;
const BrowserMutationObserver = scope.MutationObserver || scope.WebKitMutationObserver;
function makeRequestCallFromTimer(callback) {
    return function requestCall() {
        // We dispatch a timeout with a specified delay of 0 for engines that
        // can reliably accommodate that request. This will usually be snapped
        // to a 4 milisecond delay, but once we're flushing, there's no delay
        // between events.
        const timeoutHandle = setTimeout(handleTimer, 0);
        // However, since this timer gets frequently dropped in Firefox
        // workers, we enlist an interval handle that will try to fire
        // an event 20 times per second until it succeeds.
        const intervalHandle = setInterval(handleTimer, 50);
        function handleTimer() {
            // Whichever timer succeeds will cancel both timers and
            // execute the callback.
            clearTimeout(timeoutHandle);
            clearInterval(intervalHandle);
            callback();
        }
    };
}
// To request a high priority event, we induce a mutation observer by toggling
// the text of a text node between "1" and "-1".
function makeRequestCallFromMutationObserver(callback) {
    let toggle = 1;
    const observer = new BrowserMutationObserver(callback);
    const node = document.createTextNode('');
    observer.observe(node, {
        characterData: true
    });
    return function requestCall() {
        toggle = -toggle;
        node.data = toggle;
    };
}
const makeRequestCall = typeof BrowserMutationObserver === 'function' ? // reliably everywhere they are implemented.
// They are implemented in all modern browsers.
//
// - Android 4-4.3
// - Chrome 26-34
// - Firefox 14-29
// - Internet Explorer 11
// - iPad Safari 6-7.1
// - iPhone Safari 7-7.1
// - Safari 6-7
makeRequestCallFromMutationObserver : // task queue, are implemented in Internet Explorer 10, Safari 5.0-1, and Opera
// 11-12, and in web workers in many engines.
// Although message channels yield to any queued rendering and IO tasks, they
// would be better than imposing the 4ms delay of timers.
// However, they do not work reliably in Internet Explorer or Safari.
// Internet Explorer 10 is the only browser that has setImmediate but does
// not have MutationObservers.
// Although setImmediate yields to the browser's renderer, it would be
// preferrable to falling back to setTimeout since it does not have
// the minimum 4ms penalty.
// Unfortunately there appears to be a bug in Internet Explorer 10 Mobile (and
// Desktop to a lesser extent) that renders both setImmediate and
// MessageChannel useless for the purposes of ASAP.
// https://github.com/kriskowal/q/issues/396
// Timers are implemented universally.
// We fall back to timers in workers in most engines, and in foreground
// contexts in the following browsers.
// However, note that even this simple case requires nuances to operate in a
// broad spectrum of browsers.
//
// - Firefox 3-13
// - Internet Explorer 6-9
// - iPad Safari 4.3
// - Lynx 2.8.7
makeRequestCallFromTimer;

//# sourceMappingURL=makeRequestCall.mjs.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@react-dnd+asap@4.0.1/node_modules/@react-dnd/asap/dist/esm/AsapQueue.mjs

class AsapQueue {
    // Use the fastest means possible to execute a task in its own turn, with
    // priority over other events including IO, animation, reflow, and redraw
    // events in browsers.
    //
    // An exception thrown by a task will permanently interrupt the processing of
    // subsequent tasks. The higher level `asap` function ensures that if an
    // exception is thrown by a task, that the task queue will continue flushing as
    // soon as possible, but if you use `rawAsap` directly, you are responsible to
    // either ensure that no exceptions are thrown from your task, or to manually
    // call `rawAsap.requestFlush` if an exception is thrown.
    enqueueTask(task) {
        const { queue: q , requestFlush  } = this;
        if (!q.length) {
            requestFlush();
            this.flushing = true;
        }
        // Equivalent to push, but avoids a function call.
        q[q.length] = task;
    }
    constructor(){
        this.queue = [];
        // We queue errors to ensure they are thrown in right order (FIFO).
        // Array-as-queue is good enough here, since we are just dealing with exceptions.
        this.pendingErrors = [];
        // Once a flush has been requested, no further calls to `requestFlush` are
        // necessary until the next `flush` completes.
        // @ts-ignore
        this.flushing = false;
        // The position of the next task to execute in the task queue. This is
        // preserved between calls to `flush` so that it can be resumed if
        // a task throws an exception.
        this.index = 0;
        // If a task schedules additional tasks recursively, the task queue can grow
        // unbounded. To prevent memory exhaustion, the task queue will periodically
        // truncate already-completed tasks.
        this.capacity = 1024;
        // The flush function processes all tasks that have been scheduled with
        // `rawAsap` unless and until one of those tasks throws an exception.
        // If a task throws an exception, `flush` ensures that its state will remain
        // consistent and will resume where it left off when called again.
        // However, `flush` does not make any arrangements to be called again if an
        // exception is thrown.
        this.flush = ()=>{
            const { queue: q  } = this;
            while(this.index < q.length){
                const currentIndex = this.index;
                // Advance the index before calling the task. This ensures that we will
                // begin flushing on the next task the task throws an error.
                this.index++;
                q[currentIndex].call();
                // Prevent leaking memory for long chains of recursive calls to `asap`.
                // If we call `asap` within tasks scheduled by `asap`, the queue will
                // grow, but to avoid an O(n) walk for every task we execute, we don't
                // shift tasks off the queue after they have been executed.
                // Instead, we periodically shift 1024 tasks off the queue.
                if (this.index > this.capacity) {
                    // Manually shift all values starting at the index back to the
                    // beginning of the queue.
                    for(let scan = 0, newLength = q.length - this.index; scan < newLength; scan++){
                        q[scan] = q[scan + this.index];
                    }
                    q.length -= this.index;
                    this.index = 0;
                }
            }
            q.length = 0;
            this.index = 0;
            this.flushing = false;
        };
        // In a web browser, exceptions are not fatal. However, to avoid
        // slowing down the queue of pending tasks, we rethrow the error in a
        // lower priority turn.
        this.registerPendingError = (err)=>{
            this.pendingErrors.push(err);
            this.requestErrorThrow();
        };
        // `requestFlush` requests that the high priority event queue be flushed as
        // soon as possible.
        // This is useful to prevent an error thrown in a task from stalling the event
        // queue if the exception handled by Node.js’s
        // `process.on("uncaughtException")` or by a domain.
        // `requestFlush` is implemented using a strategy based on data collected from
        // every available SauceLabs Selenium web driver worker at time of writing.
        // https://docs.google.com/spreadsheets/d/1mG-5UYGup5qxGdEMWkhP6BWCz053NUb2E1QoUTU16uA/edit#gid=783724593
        this.requestFlush = makeRequestCall(this.flush);
        this.requestErrorThrow = makeRequestCallFromTimer(()=>{
            // Throw first error
            if (this.pendingErrors.length) {
                throw this.pendingErrors.shift();
            }
        });
    }
} // The message channel technique was discovered by Malte Ubl and was the
 // original foundation for this library.
 // http://www.nonblocking.io/2011/06/windownexttick.html
 // Safari 6.0.5 (at least) intermittently fails to create message ports on a
 // page's first load. Thankfully, this version of Safari supports
 // MutationObservers, so we don't need to fall back in that case.
 // function makeRequestCallFromMessageChannel(callback) {
 //     var channel = new MessageChannel();
 //     channel.port1.onmessage = callback;
 //     return function requestCall() {
 //         channel.port2.postMessage(0);
 //     };
 // }
 // For reasons explained above, we are also unable to use `setImmediate`
 // under any circumstances.
 // Even if we were, there is another bug in Internet Explorer 10.
 // It is not sufficient to assign `setImmediate` to `requestFlush` because
 // `setImmediate` must be called *by name* and therefore must be wrapped in a
 // closure.
 // Never forget.
 // function makeRequestCallFromSetImmediate(callback) {
 //     return function requestCall() {
 //         setImmediate(callback);
 //     };
 // }
 // Safari 6.0 has a problem where timers will get lost while the user is
 // scrolling. This problem does not impact ASAP because Safari 6.0 supports
 // mutation observers, so that implementation is used instead.
 // However, if we ever elect to use timers in Safari, the prevalent work-around
 // is to add a scroll event listener that calls for a flush.
 // `setTimeout` does not call the passed callback if the delay is less than
 // approximately 7 in web workers in Firefox 8 through 18, and sometimes not
 // even then.
 // This is for `asap.js` only.
 // Its name will be periodically randomized to break any code that depends on
 // // its existence.
 // rawAsap.makeRequestCallFromTimer = makeRequestCallFromTimer
 // ASAP was originally a nextTick shim included in Q. This was factored out
 // into this ASAP package. It was later adapted to RSVP which made further
 // amendments. These decisions, particularly to marginalize MessageChannel and
 // to capture the MutationObserver implementation in a closure, were integrated
 // back into ASAP proper.
 // https://github.com/tildeio/rsvp.js/blob/cddf7232546a9cf858524b75cde6f9edf72620a7/lib/rsvp/asap.js

//# sourceMappingURL=AsapQueue.mjs.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@react-dnd+asap@4.0.1/node_modules/@react-dnd/asap/dist/esm/RawTask.mjs
// `call`, just like a function.
class RawTask {
    call() {
        try {
            this.task && this.task();
        } catch (error) {
            this.onError(error);
        } finally{
            this.task = null;
            this.release(this);
        }
    }
    constructor(onError, release){
        this.onError = onError;
        this.release = release;
        this.task = null;
    }
}

//# sourceMappingURL=RawTask.mjs.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@react-dnd+asap@4.0.1/node_modules/@react-dnd/asap/dist/esm/TaskFactory.mjs

class TaskFactory {
    create(task) {
        const tasks = this.freeTasks;
        const t1 = tasks.length ? tasks.pop() : new RawTask(this.onError, (t)=>tasks[tasks.length] = t
        );
        t1.task = task;
        return t1;
    }
    constructor(onError){
        this.onError = onError;
        this.freeTasks = [];
    }
}

//# sourceMappingURL=TaskFactory.mjs.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@react-dnd+asap@4.0.1/node_modules/@react-dnd/asap/dist/esm/asap.mjs


const asapQueue = new AsapQueue();
const taskFactory = new TaskFactory(asapQueue.registerPendingError);
/**
 * Calls a task as soon as possible after returning, in its own event, with priority
 * over other events like animation, reflow, and repaint. An error thrown from an
 * event will not interrupt, nor even substantially slow down the processing of
 * other events, but will be rather postponed to a lower priority event.
 * @param {{call}} task A callable object, typically a function that takes no
 * arguments.
 */ function asap(task) {
    asapQueue.enqueueTask(taskFactory.create(task));
}

//# sourceMappingURL=asap.mjs.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@react-dnd+asap@4.0.1/node_modules/@react-dnd/asap/dist/esm/index.mjs





//# sourceMappingURL=index.mjs.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/classes/HandlerRegistryImpl.js
function HandlerRegistryImpl_classCallCheck(instance, Constructor) { if (!(instance instanceof Constructor)) { throw new TypeError("Cannot call a class as a function"); } }

function HandlerRegistryImpl_defineProperties(target, props) { for (var i = 0; i < props.length; i++) { var descriptor = props[i]; descriptor.enumerable = descriptor.enumerable || false; descriptor.configurable = true; if ("value" in descriptor) descriptor.writable = true; Object.defineProperty(target, descriptor.key, descriptor); } }

function HandlerRegistryImpl_createClass(Constructor, protoProps, staticProps) { if (protoProps) HandlerRegistryImpl_defineProperties(Constructor.prototype, protoProps); if (staticProps) HandlerRegistryImpl_defineProperties(Constructor, staticProps); return Constructor; }

function HandlerRegistryImpl_defineProperty(obj, key, value) { if (key in obj) { Object.defineProperty(obj, key, { value: value, enumerable: true, configurable: true, writable: true }); } else { obj[key] = value; } return obj; }

function HandlerRegistryImpl_slicedToArray(arr, i) { return HandlerRegistryImpl_arrayWithHoles(arr) || HandlerRegistryImpl_iterableToArrayLimit(arr, i) || HandlerRegistryImpl_unsupportedIterableToArray(arr, i) || HandlerRegistryImpl_nonIterableRest(); }

function HandlerRegistryImpl_nonIterableRest() { throw new TypeError("Invalid attempt to destructure non-iterable instance.\nIn order to be iterable, non-array objects must have a [Symbol.iterator]() method."); }

function HandlerRegistryImpl_unsupportedIterableToArray(o, minLen) { if (!o) return; if (typeof o === "string") return HandlerRegistryImpl_arrayLikeToArray(o, minLen); var n = Object.prototype.toString.call(o).slice(8, -1); if (n === "Object" && o.constructor) n = o.constructor.name; if (n === "Map" || n === "Set") return Array.from(o); if (n === "Arguments" || /^(?:Ui|I)nt(?:8|16|32)(?:Clamped)?Array$/.test(n)) return HandlerRegistryImpl_arrayLikeToArray(o, minLen); }

function HandlerRegistryImpl_arrayLikeToArray(arr, len) { if (len == null || len > arr.length) len = arr.length; for (var i = 0, arr2 = new Array(len); i < len; i++) { arr2[i] = arr[i]; } return arr2; }

function HandlerRegistryImpl_iterableToArrayLimit(arr, i) { var _i = arr == null ? null : typeof Symbol !== "undefined" && arr[Symbol.iterator] || arr["@@iterator"]; if (_i == null) return; var _arr = []; var _n = true; var _d = false; var _s, _e; try { for (_i = _i.call(arr); !(_n = (_s = _i.next()).done); _n = true) { _arr.push(_s.value); if (i && _arr.length === i) break; } } catch (err) { _d = true; _e = err; } finally { try { if (!_n && _i["return"] != null) _i["return"](); } finally { if (_d) throw _e; } } return _arr; }

function HandlerRegistryImpl_arrayWithHoles(arr) { if (Array.isArray(arr)) return arr; }








function getNextHandlerId(role) {
  var id = getNextUniqueId().toString();

  switch (role) {
    case HandlerRole.SOURCE:
      return "S".concat(id);

    case HandlerRole.TARGET:
      return "T".concat(id);

    default:
      throw new Error("Unknown Handler Role: ".concat(role));
  }
}

function parseRoleFromHandlerId(handlerId) {
  switch (handlerId[0]) {
    case 'S':
      return HandlerRole.SOURCE;

    case 'T':
      return HandlerRole.TARGET;

    default:
      invariant(false, "Cannot parse handler ID: ".concat(handlerId));
  }
}

function mapContainsValue(map, searchValue) {
  var entries = map.entries();
  var isDone = false;

  do {
    var _entries$next = entries.next(),
        done = _entries$next.done,
        _entries$next$value = HandlerRegistryImpl_slicedToArray(_entries$next.value, 2),
        value = _entries$next$value[1];

    if (value === searchValue) {
      return true;
    }

    isDone = !!done;
  } while (!isDone);

  return false;
}

var HandlerRegistryImpl = /*#__PURE__*/function () {
  function HandlerRegistryImpl(store) {
    HandlerRegistryImpl_classCallCheck(this, HandlerRegistryImpl);

    HandlerRegistryImpl_defineProperty(this, "types", new Map());

    HandlerRegistryImpl_defineProperty(this, "dragSources", new Map());

    HandlerRegistryImpl_defineProperty(this, "dropTargets", new Map());

    HandlerRegistryImpl_defineProperty(this, "pinnedSourceId", null);

    HandlerRegistryImpl_defineProperty(this, "pinnedSource", null);

    HandlerRegistryImpl_defineProperty(this, "store", void 0);

    this.store = store;
  }

  HandlerRegistryImpl_createClass(HandlerRegistryImpl, [{
    key: "addSource",
    value: function addSource(type, source) {
      validateType(type);
      validateSourceContract(source);
      var sourceId = this.addHandler(HandlerRole.SOURCE, type, source);
      this.store.dispatch(registry_addSource(sourceId));
      return sourceId;
    }
  }, {
    key: "addTarget",
    value: function addTarget(type, target) {
      validateType(type, true);
      validateTargetContract(target);
      var targetId = this.addHandler(HandlerRole.TARGET, type, target);
      this.store.dispatch(registry_addTarget(targetId));
      return targetId;
    }
  }, {
    key: "containsHandler",
    value: function containsHandler(handler) {
      return mapContainsValue(this.dragSources, handler) || mapContainsValue(this.dropTargets, handler);
    }
  }, {
    key: "getSource",
    value: function getSource(sourceId) {
      var includePinned = arguments.length > 1 && arguments[1] !== undefined ? arguments[1] : false;
      invariant(this.isSourceId(sourceId), 'Expected a valid source ID.');
      var isPinned = includePinned && sourceId === this.pinnedSourceId;
      var source = isPinned ? this.pinnedSource : this.dragSources.get(sourceId);
      return source;
    }
  }, {
    key: "getTarget",
    value: function getTarget(targetId) {
      invariant(this.isTargetId(targetId), 'Expected a valid target ID.');
      return this.dropTargets.get(targetId);
    }
  }, {
    key: "getSourceType",
    value: function getSourceType(sourceId) {
      invariant(this.isSourceId(sourceId), 'Expected a valid source ID.');
      return this.types.get(sourceId);
    }
  }, {
    key: "getTargetType",
    value: function getTargetType(targetId) {
      invariant(this.isTargetId(targetId), 'Expected a valid target ID.');
      return this.types.get(targetId);
    }
  }, {
    key: "isSourceId",
    value: function isSourceId(handlerId) {
      var role = parseRoleFromHandlerId(handlerId);
      return role === HandlerRole.SOURCE;
    }
  }, {
    key: "isTargetId",
    value: function isTargetId(handlerId) {
      var role = parseRoleFromHandlerId(handlerId);
      return role === HandlerRole.TARGET;
    }
  }, {
    key: "removeSource",
    value: function removeSource(sourceId) {
      var _this = this;

      invariant(this.getSource(sourceId), 'Expected an existing source.');
      this.store.dispatch(registry_removeSource(sourceId));
      asap(function () {
        _this.dragSources.delete(sourceId);

        _this.types.delete(sourceId);
      });
    }
  }, {
    key: "removeTarget",
    value: function removeTarget(targetId) {
      invariant(this.getTarget(targetId), 'Expected an existing target.');
      this.store.dispatch(registry_removeTarget(targetId));
      this.dropTargets.delete(targetId);
      this.types.delete(targetId);
    }
  }, {
    key: "pinSource",
    value: function pinSource(sourceId) {
      var source = this.getSource(sourceId);
      invariant(source, 'Expected an existing source.');
      this.pinnedSourceId = sourceId;
      this.pinnedSource = source;
    }
  }, {
    key: "unpinSource",
    value: function unpinSource() {
      invariant(this.pinnedSource, 'No source is pinned at the time.');
      this.pinnedSourceId = null;
      this.pinnedSource = null;
    }
  }, {
    key: "addHandler",
    value: function addHandler(role, type, handler) {
      var id = getNextHandlerId(role);
      this.types.set(id, type);

      if (role === HandlerRole.SOURCE) {
        this.dragSources.set(id, handler);
      } else if (role === HandlerRole.TARGET) {
        this.dropTargets.set(id, handler);
      }

      return id;
    }
  }]);

  return HandlerRegistryImpl;
}();
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/dnd-core@14.0.1/node_modules/dnd-core/dist/esm/createDragDropManager.js





function createDragDropManager(backendFactory) {
  var globalContext = arguments.length > 1 && arguments[1] !== undefined ? arguments[1] : undefined;
  var backendOptions = arguments.length > 2 && arguments[2] !== undefined ? arguments[2] : {};
  var debugMode = arguments.length > 3 && arguments[3] !== undefined ? arguments[3] : false;
  var store = makeStoreInstance(debugMode);
  var monitor = new DragDropMonitorImpl(store, new HandlerRegistryImpl(store));
  var manager = new DragDropManagerImpl(store, monitor);
  var backend = backendFactory(manager, globalContext, backendOptions);
  manager.receiveBackend(backend);
  return manager;
}

function makeStoreInstance(debugMode) {
  // TODO: if we ever make a react-native version of this,
  // we'll need to consider how to pull off dev-tooling
  var reduxDevTools = typeof window !== 'undefined' && window.__REDUX_DEVTOOLS_EXTENSION__;
  return createStore(reducers_reduce, debugMode && reduxDevTools && reduxDevTools({
    name: 'dnd-core',
    instanceId: 'dnd-core'
  }));
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/core/DndProvider.js
var DndProvider_excluded = ["children"];

function DndProvider_slicedToArray(arr, i) { return DndProvider_arrayWithHoles(arr) || DndProvider_iterableToArrayLimit(arr, i) || DndProvider_unsupportedIterableToArray(arr, i) || DndProvider_nonIterableRest(); }

function DndProvider_nonIterableRest() { throw new TypeError("Invalid attempt to destructure non-iterable instance.\nIn order to be iterable, non-array objects must have a [Symbol.iterator]() method."); }

function DndProvider_unsupportedIterableToArray(o, minLen) { if (!o) return; if (typeof o === "string") return DndProvider_arrayLikeToArray(o, minLen); var n = Object.prototype.toString.call(o).slice(8, -1); if (n === "Object" && o.constructor) n = o.constructor.name; if (n === "Map" || n === "Set") return Array.from(o); if (n === "Arguments" || /^(?:Ui|I)nt(?:8|16|32)(?:Clamped)?Array$/.test(n)) return DndProvider_arrayLikeToArray(o, minLen); }

function DndProvider_arrayLikeToArray(arr, len) { if (len == null || len > arr.length) len = arr.length; for (var i = 0, arr2 = new Array(len); i < len; i++) { arr2[i] = arr[i]; } return arr2; }

function DndProvider_iterableToArrayLimit(arr, i) { var _i = arr == null ? null : typeof Symbol !== "undefined" && arr[Symbol.iterator] || arr["@@iterator"]; if (_i == null) return; var _arr = []; var _n = true; var _d = false; var _s, _e; try { for (_i = _i.call(arr); !(_n = (_s = _i.next()).done); _n = true) { _arr.push(_s.value); if (i && _arr.length === i) break; } } catch (err) { _d = true; _e = err; } finally { try { if (!_n && _i["return"] != null) _i["return"](); } finally { if (_d) throw _e; } } return _arr; }

function DndProvider_arrayWithHoles(arr) { if (Array.isArray(arr)) return arr; }

function _objectWithoutProperties(source, excluded) { if (source == null) return {}; var target = DndProvider_objectWithoutPropertiesLoose(source, excluded); var key, i; if (Object.getOwnPropertySymbols) { var sourceSymbolKeys = Object.getOwnPropertySymbols(source); for (i = 0; i < sourceSymbolKeys.length; i++) { key = sourceSymbolKeys[i]; if (excluded.indexOf(key) >= 0) continue; if (!Object.prototype.propertyIsEnumerable.call(source, key)) continue; target[key] = source[key]; } } return target; }

function DndProvider_objectWithoutPropertiesLoose(source, excluded) { if (source == null) return {}; var target = {}; var sourceKeys = Object.keys(source); var key, i; for (i = 0; i < sourceKeys.length; i++) { key = sourceKeys[i]; if (excluded.indexOf(key) >= 0) continue; target[key] = source[key]; } return target; }





var refCount = 0;
var INSTANCE_SYM = Symbol.for('__REACT_DND_CONTEXT_INSTANCE__');
/**
 * A React component that provides the React-DnD context
 */

var DndProvider = (0,react.memo)(function DndProvider(_ref) {
  var children = _ref.children,
      props = _objectWithoutProperties(_ref, DndProvider_excluded);

  var _getDndContextValue = getDndContextValue(props),
      _getDndContextValue2 = DndProvider_slicedToArray(_getDndContextValue, 2),
      manager = _getDndContextValue2[0],
      isGlobalInstance = _getDndContextValue2[1]; // memoized from props

  /**
   * If the global context was used to store the DND context
   * then where theres no more references to it we should
   * clean it up to avoid memory leaks
   */


  (0,react.useEffect)(function () {
    if (isGlobalInstance) {
      var context = getGlobalContext();
      ++refCount;
      return function () {
        if (--refCount === 0) {
          context[INSTANCE_SYM] = null;
        }
      };
    }
  }, []);
  return (0,react_jsx_runtime.jsx)(DndContext_DndContext.Provider, Object.assign({
    value: manager
  }, {
    children: children
  }), void 0);
});

function getDndContextValue(props) {
  if ('manager' in props) {
    var _manager = {
      dragDropManager: props.manager
    };
    return [_manager, false];
  }

  var manager = createSingletonDndContext(props.backend, props.context, props.options, props.debugMode);
  var isGlobalInstance = !props.context;
  return [manager, isGlobalInstance];
}

function createSingletonDndContext(backend) {
  var context = arguments.length > 1 && arguments[1] !== undefined ? arguments[1] : getGlobalContext();
  var options = arguments.length > 2 ? arguments[2] : undefined;
  var debugMode = arguments.length > 3 ? arguments[3] : undefined;
  var ctx = context;

  if (!ctx[INSTANCE_SYM]) {
    ctx[INSTANCE_SYM] = {
      dragDropManager: createDragDropManager(backend, context, options, debugMode)
    };
  }

  return ctx[INSTANCE_SYM];
}

function getGlobalContext() {
  return typeof global !== 'undefined' ? global : window;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/provider.js











const SERVER_STATE = initialState();
function TreeProvider({ treeProps, imperativeHandle, children, }) {
    const list = (0,react.useRef)(null);
    const listEl = (0,react.useRef)(null);
    const store = (0,react.useRef)(
    // @ts-ignore
    (0,redux/* createStore */.y$)(rootReducer, initialState(treeProps)));
    const state = (0,shim.useSyncExternalStore)(store.current.subscribe, store.current.getState, () => SERVER_STATE);
    /* The tree api object is stable. */
    const api = (0,react.useMemo)(() => {
        return new TreeApi(store.current, treeProps, list, listEl);
    }, []);
    /* Make sure the tree instance stays in sync */
    const updateCount = (0,react.useRef)(0);
    (0,react.useMemo)(() => {
        updateCount.current += 1;
        api.update(treeProps);
    }, [...Object.values(treeProps), state.nodes.open]);
    /* Expose the tree api */
    (0,react.useImperativeHandle)(imperativeHandle, () => api);
    /* Change selection based on props */
    (0,react.useEffect)(() => {
        if (api.props.selection) {
            api.select(api.props.selection, { focus: false });
        }
        else {
            api.deselectAll();
        }
    }, [api.props.selection]);
    /* Clear visability for filtered nodes */
    (0,react.useEffect)(() => {
        if (!api.props.searchTerm) {
            store.current.dispatch(actions.clear(true));
        }
    }, [api.props.searchTerm]);
    return ((0,react_jsx_runtime.jsx)(TreeApiContext.Provider, { value: api, children: (0,react_jsx_runtime.jsx)(DataUpdatesContext.Provider, { value: updateCount.current, children: (0,react_jsx_runtime.jsx)(NodesContext.Provider, { value: state.nodes, children: (0,react_jsx_runtime.jsx)(DndContext.Provider, { value: state.dnd, children: (0,react_jsx_runtime.jsx)(DndProvider, Object.assign({ backend: HTML5Backend, options: { rootElement: api.props.dndRootElement || undefined } }, (treeProps.dndManager && { manager: treeProps.dndManager }), { children: children })) }) }) }) }));
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/dnd/outer-drop-hook.js




function useOuterDrop() {
    const tree = useTreeApi();
    // In case we drop an item at the bottom of the list
    const [, drop] = useDrop(() => ({
        accept: "NODE",
        canDrop: (_item, m) => {
            if (!m.isOver({ shallow: true }))
                return false;
            return tree.canDrop();
        },
        hover: (_item, m) => {
            if (!m.isOver({ shallow: true }))
                return;
            const offset = m.getClientOffset();
            if (!tree.listEl.current || !offset)
                return;
            const { cursor, drop } = computeDrop({
                element: tree.listEl.current,
                offset: offset,
                indent: tree.indent,
                node: null,
                prevNode: tree.visibleNodes[tree.visibleNodes.length - 1],
                nextNode: null,
            });
            if (drop)
                tree.dispatch(dnd_slice_actions.hovering(drop.parentId, drop.index));
            if (m.canDrop()) {
                if (cursor)
                    tree.showCursor(cursor);
            }
            else {
                tree.hideCursor();
            }
        },
    }), [tree]);
    drop(tree.listEl);
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/outer-drop.js

function OuterDrop(props) {
    useOuterDrop();
    return props.children;
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/tree-container.js



function TreeContainer() {
    const tree = useTreeApi();
    const Container = tree.props.renderContainer || DefaultContainer;
    return ((0,react_jsx_runtime.jsx)(react_jsx_runtime.Fragment, { children: (0,react_jsx_runtime.jsx)(Container, {}) }));
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-dnd@14.0.5+388b6158efb4beb1/node_modules/react-dnd/dist/esm/hooks/useDragLayer.js
function useDragLayer_slicedToArray(arr, i) { return useDragLayer_arrayWithHoles(arr) || useDragLayer_iterableToArrayLimit(arr, i) || useDragLayer_unsupportedIterableToArray(arr, i) || useDragLayer_nonIterableRest(); }

function useDragLayer_nonIterableRest() { throw new TypeError("Invalid attempt to destructure non-iterable instance.\nIn order to be iterable, non-array objects must have a [Symbol.iterator]() method."); }

function useDragLayer_unsupportedIterableToArray(o, minLen) { if (!o) return; if (typeof o === "string") return useDragLayer_arrayLikeToArray(o, minLen); var n = Object.prototype.toString.call(o).slice(8, -1); if (n === "Object" && o.constructor) n = o.constructor.name; if (n === "Map" || n === "Set") return Array.from(o); if (n === "Arguments" || /^(?:Ui|I)nt(?:8|16|32)(?:Clamped)?Array$/.test(n)) return useDragLayer_arrayLikeToArray(o, minLen); }

function useDragLayer_arrayLikeToArray(arr, len) { if (len == null || len > arr.length) len = arr.length; for (var i = 0, arr2 = new Array(len); i < len; i++) { arr2[i] = arr[i]; } return arr2; }

function useDragLayer_iterableToArrayLimit(arr, i) { var _i = arr == null ? null : typeof Symbol !== "undefined" && arr[Symbol.iterator] || arr["@@iterator"]; if (_i == null) return; var _arr = []; var _n = true; var _d = false; var _s, _e; try { for (_i = _i.call(arr); !(_n = (_s = _i.next()).done); _n = true) { _arr.push(_s.value); if (i && _arr.length === i) break; } } catch (err) { _d = true; _e = err; } finally { try { if (!_n && _i["return"] != null) _i["return"](); } finally { if (_d) throw _e; } } return _arr; }

function useDragLayer_arrayWithHoles(arr) { if (Array.isArray(arr)) return arr; }




/**
 * useDragLayer Hook
 * @param collector The property collector
 */

function useDragLayer(collect) {
  var dragDropManager = useDragDropManager();
  var monitor = dragDropManager.getMonitor();

  var _useCollector = useCollector(monitor, collect),
      _useCollector2 = useDragLayer_slicedToArray(_useCollector, 2),
      collected = _useCollector2[0],
      updateCollected = _useCollector2[1];

  (0,react.useEffect)(function () {
    return monitor.subscribeToOffsetChange(updateCollected);
  });
  (0,react.useEffect)(function () {
    return monitor.subscribeToStateChange(updateCollected);
  });
  return collected;
}
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/drag-preview-container.js




function DragPreviewContainer() {
    const tree = useTreeApi();
    const { offset, mouse, item, isDragging } = useDragLayer((m) => {
        return {
            offset: m.getSourceClientOffset(),
            mouse: m.getClientOffset(),
            item: m.getItem(),
            isDragging: m.isDragging(),
        };
    });
    const DragPreview = tree.props.renderDragPreview || DefaultDragPreview;
    return ((0,react_jsx_runtime.jsx)(DragPreview, { offset: offset, mouse: mouse, id: (item === null || item === void 0 ? void 0 : item.id) || null, dragIds: (item === null || item === void 0 ? void 0 : item.dragIds) || [], isDragging: isDragging }));
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/data/simple-tree.js
class SimpleTree {
    constructor(data) {
        this.root = simple_tree_createRoot(data);
    }
    get data() {
        var _a, _b;
        return (_b = (_a = this.root.children) === null || _a === void 0 ? void 0 : _a.map((node) => node.data)) !== null && _b !== void 0 ? _b : [];
    }
    create(args) {
        const parent = args.parentId ? this.find(args.parentId) : this.root;
        if (!parent)
            return null;
        parent.addChild(args.data, args.index);
    }
    move(args) {
        const src = this.find(args.id);
        const parent = args.parentId ? this.find(args.parentId) : this.root;
        if (!src || !parent)
            return;
        parent.addChild(src.data, args.index);
        src.drop();
    }
    update(args) {
        const node = this.find(args.id);
        if (node)
            node.update(args.changes);
    }
    drop(args) {
        const node = this.find(args.id);
        if (node)
            node.drop();
    }
    find(id, node = this.root) {
        if (!node)
            return null;
        if (node.id === id)
            return node;
        if (node.children) {
            for (let child of node.children) {
                const found = this.find(id, child);
                if (found)
                    return found;
            }
            return null;
        }
        return null;
    }
}
function simple_tree_createRoot(data) {
    const root = new SimpleNode({ id: "ROOT" }, null);
    root.children = data.map((d) => createNode(d, root));
    return root;
}
function createNode(data, parent) {
    const node = new SimpleNode(data, parent);
    if (data.children)
        node.children = data.children.map((d) => createNode(d, node));
    return node;
}
class SimpleNode {
    constructor(data, parent) {
        this.data = data;
        this.parent = parent;
        this.id = data.id;
    }
    hasParent() {
        return !!this.parent;
    }
    get childIndex() {
        return this.hasParent() ? this.parent.children.indexOf(this) : -1;
    }
    addChild(data, index) {
        var _a, _b;
        const node = createNode(data, this);
        this.children = (_a = this.children) !== null && _a !== void 0 ? _a : [];
        this.children.splice(index, 0, node);
        this.data.children = (_b = this.data.children) !== null && _b !== void 0 ? _b : [];
        this.data.children.splice(index, 0, data);
    }
    removeChild(index) {
        var _a, _b;
        (_a = this.children) === null || _a === void 0 ? void 0 : _a.splice(index, 1);
        (_b = this.data.children) === null || _b === void 0 ? void 0 : _b.splice(index, 1);
    }
    update(changes) {
        if (this.hasParent()) {
            const i = this.childIndex;
            this.parent.addChild(Object.assign(Object.assign({}, this.data), changes), i);
            this.drop();
        }
    }
    drop() {
        if (this.hasParent())
            this.parent.removeChild(this.childIndex);
    }
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/hooks/use-simple-tree.js


let nextId = 0;
function useSimpleTree(initialData) {
    const [data, setData] = (0,react.useState)(initialData);
    const tree = (0,react.useMemo)(() => new SimpleTree(data), [data]);
    const onMove = (args) => {
        for (const id of args.dragIds) {
            tree.move({ id, parentId: args.parentId, index: args.index });
        }
        setData(tree.data);
    };
    const onRename = ({ name, id }) => {
        tree.update({ id, changes: { name } });
        setData(tree.data);
    };
    const onCreate = ({ parentId, index, type }) => {
        const data = { id: `simple-tree-id-${nextId++}`, name: "" };
        if (type === "internal")
            data.children = [];
        tree.create({ parentId, index, data });
        setData(tree.data);
        return data;
    };
    const onDelete = (args) => {
        args.ids.forEach((id) => tree.drop({ id }));
        setData(tree.data);
    };
    const controller = { onMove, onRename, onCreate, onDelete };
    return [data, controller];
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/hooks/use-validated-props.js

function useValidatedProps(props) {
    if (props.initialData && props.data) {
        throw new Error(`React Arborist Tree => Provide either a data or initialData prop, but not both.`);
    }
    if (props.initialData &&
        (props.onCreate || props.onDelete || props.onMove || props.onRename)) {
        throw new Error(`React Arborist Tree => You passed the initialData prop along with a data handler.
Use the data prop if you want to provide your own handlers.`);
    }
    if (props.initialData) {
        /**
         * Let's break the rules of hooks here. If the initialData prop
         * is provided, we will assume it will not change for the life of
         * the component.
         *
         * We will provide the real data and the handlers to update it.
         *   */
        const [data, controller] = useSimpleTree(props.initialData);
        return Object.assign(Object.assign(Object.assign({}, props), controller), { data });
    }
    else {
        return props;
    }
}

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/react-arborist@3.4.3+3da047711fb244d7/node_modules/react-arborist/dist/module/components/tree.js







function TreeComponent(props, ref) {
    const treeProps = useValidatedProps(props);
    return ((0,react_jsx_runtime.jsxs)(TreeProvider, { treeProps: treeProps, imperativeHandle: ref, children: [(0,react_jsx_runtime.jsx)(OuterDrop, { children: (0,react_jsx_runtime.jsx)(TreeContainer, {}) }), (0,react_jsx_runtime.jsx)(DragPreviewContainer, {})] }));
}
const Tree = (0,react.forwardRef)(TreeComponent);


/***/ }),

/***/ 77475:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._1fx45750 {\n  display: flex;\n  align-items: flex-start;\n  gap: var(--_19ez2rz2z);\n  padding: var(--_19ez2rz30);\n  border: 1px solid var(--_19ez2rzm);\n  border-radius: var(--_19ez2rz37);\n  background: var(--_19ez2rz17);\n  color: var(--_19ez2rz0);\n  font-size: var(--_19ez2rz3b);\n}\n._1fx45751 {\n  color: var(--_19ez2rz19);\n  flex-shrink: 0;\n  line-height: var(--_19ez2rz3d);\n}\n._1fx45752 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2z);\n  flex: 1 1;\n  min-width: 0;\n}\n._1fx45753 {\n  color: var(--_19ez2rz0);\n}\n._1fx45754 {\n  display: flex;\n  gap: var(--_19ez2rz2z);\n  flex-wrap: wrap;\n}\n._1fx45755 {\n  background: none;\n  border: none;\n  cursor: pointer;\n  color: var(--_19ez2rz19);\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3g);\n  padding: 0;\n  text-decoration: underline;\n}\n._1fx45755:hover {\n  color: var(--_19ez2rzn);\n}\n._1fx45756 {\n  background: var(--_19ez2rzm);\n  border: none;\n  border-radius: var(--_19ez2rz36);\n  cursor: pointer;\n  color: var(--_19ez2rzq);\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3g);\n  padding: var(--_19ez2rz2y) var(--_19ez2rz30);\n}\n._1fx45756:hover {\n  background: var(--_19ez2rzn);\n}\n._1fx45757 {\n  background: none;\n  border: none;\n  cursor: pointer;\n  color: var(--_19ez2rz2);\n  font-size: var(--_19ez2rz3b);\n  padding: 2px;\n  margin-left: auto;\n  line-height: 1;\n  min-width: 24px;\n  min-height: 24px;\n  display: flex;\n  align-items: center;\n  justify-content: center;\n  flex-shrink: 0;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 78064:
/***/ ((module) => {



// do not edit .js files directly - edit src/index.jst



module.exports = function equal(a, b) {
  if (a === b) return true;

  if (a && b && typeof a == 'object' && typeof b == 'object') {
    if (a.constructor !== b.constructor) return false;

    var length, i, keys;
    if (Array.isArray(a)) {
      length = a.length;
      if (length != b.length) return false;
      for (i = length; i-- !== 0;)
        if (!equal(a[i], b[i])) return false;
      return true;
    }



    if (a.constructor === RegExp) return a.source === b.source && a.flags === b.flags;
    if (a.valueOf !== Object.prototype.valueOf) return a.valueOf() === b.valueOf();
    if (a.toString !== Object.prototype.toString) return a.toString() === b.toString();

    keys = Object.keys(a);
    length = keys.length;
    if (length !== Object.keys(b).length) return false;

    for (i = length; i-- !== 0;)
      if (!Object.prototype.hasOwnProperty.call(b, keys[i])) return false;

    for (i = length; i-- !== 0;) {
      var key = keys[i];

      if (!equal(a[key], b[key])) return false;
    }

    return true;
  }

  // true if both NaN, false otherwise
  return a!==a && b!==b;
};


/***/ }),

/***/ 86510:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._465h7s0 {\n  display: flex;\n  flex-direction: column;\n  min-height: 0;\n}\n._465h7s1 {\n  display: flex;\n  flex-wrap: wrap;\n  align-items: center;\n  gap: 8px;\n  padding: 4px 8px;\n  border-bottom: 1px solid var(--_19ez2rze);\n  position: -webkit-sticky;\n  position: sticky;\n  top: 0;\n  background: var(--_19ez2rz7);\n  color: var(--_19ez2rz0);\n}\n._465h7s2 {\n  min-width: 44px;\n  min-height: 44px;\n  font: inherit;\n  padding: 0 12px;\n  cursor: pointer;\n}\n._465h7s2:focus-visible {\n  outline: 2px solid var(--_19ez2rzl);\n  outline-offset: 2px;\n}\n._465h7s2[aria-disabled='true'], ._465h7s2:disabled {\n  opacity: 0.5;\n  cursor: default;\n}\n._465h7s3 {\n  min-height: 44px;\n  width: 7em;\n  font: inherit;\n  padding: 0 8px;\n  margin-left: 6px;\n}\n._465h7s4 {\n  font-feature-settings: \"tnum\";\n  font-variant-numeric: tabular-nums;\n}\n._465h7s5 {\n  font-weight: 600;\n}\n._465h7s6 {\n  margin: 0;\n  padding: 8px;\n  font-family: var(--_19ez2rz2u);\n  font-size: 14px;\n  line-height: 1.5;\n  white-space: pre-wrap;\n  overflow-wrap: anywhere;\n}\n._465h7s6:focus-visible {\n  outline: 2px solid var(--_19ez2rzl);\n  outline-offset: -2px;\n}\n._465h7s7 {\n  display: block;\n}\n._465h7s7:focus-visible {\n  outline: 2px solid var(--_19ez2rzl);\n}\n._465h7s7[data-hit='true'] {\n  background: var(--_19ez2rz8);\n}\n._465h7s8 {\n  position: absolute;\n  left: 8px;\n  top: 4px;\n  z-index: 5;\n  min-height: 44px;\n  padding: 0 12px;\n  font: inherit;\n  transform: translateY(-200%);\n}\n._465h7s8:focus {\n  transform: none;\n}\n@media (pointer: coarse) {\n  ._465h7s3 {\n    font-size: 16px;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 87892:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._1em5m4d0 {\n  display: flex;\n  flex-direction: column;\n  height: 100%;\n  min-height: 0;\n}\n._1em5m4d1 {\n  flex: 1 1;\n  min-height: 0;\n  height: 100%;\n}\n._1em5m4d2 {\n  flex-shrink: 0;\n  padding: 4px 12px;\n  font-size: 12px;\n  background: var(--_19ez2rzy);\n  color: var(--_19ez2rzz);\n  border-bottom: 1px solid var(--_19ez2rzx);\n}\n._1em5m4d1 .cm-editor {\n  height: 100%;\n}\n._1em5m4d1 .cm-changeGutter {\n  width: 4px;\n}\n._1em5m4d3 {\n  width: 4px;\n  height: 100%;\n}\n._1em5m4d4 {\n  background: var(--_19ez2rz1z);\n}\n._1em5m4d5 {\n  background: var(--_19ez2rz20);\n}\n._1em5m4d6 {\n  background: var(--_19ez2rz1v);\n}\n._1em5m4d7 {\n  background-image: linear-gradient(var(--_19ez2rz1x), var(--_19ez2rz1x));\n}\n._1em5m4d8 {\n  background-image: linear-gradient(var(--_19ez2rz1y), var(--_19ez2rz1y));\n}\n._1em5m4d9 {\n  background-image: linear-gradient(var(--_19ez2rz1w), var(--_19ez2rz1w));\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 98136:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   A: () => (/* binding */ Info)
/* harmony export */ });
/* unused harmony export __iconNode */
/* harmony import */ var _createLucideIcon_mjs__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(46657);
/**
 * @license lucide-react v1.14.0 - ISC
 *
 * This source code is licensed under the ISC license.
 * See the LICENSE file in the root directory of this source tree.
 */ 
const __iconNode = [
    [
        "circle",
        {
            cx: "12",
            cy: "12",
            r: "10",
            key: "1mglay"
        }
    ],
    [
        "path",
        {
            d: "M12 16v-4",
            key: "1dtifu"
        }
    ],
    [
        "path",
        {
            d: "M12 8h.01",
            key: "e9boi3"
        }
    ]
];
const Info = (0,_createLucideIcon_mjs__WEBPACK_IMPORTED_MODULE_0__/* ["default"] */ .A)("info", __iconNode);
 //# sourceMappingURL=info.mjs.map


/***/ })

};
;