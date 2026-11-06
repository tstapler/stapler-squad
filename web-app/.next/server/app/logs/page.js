(() => {
var exports = {};
exports.id = 1524;
exports.ids = [1524];
exports.modules = {

/***/ 1708:
/***/ ((module) => {

"use strict";
module.exports = require("node:process");

/***/ }),

/***/ 3295:
/***/ ((module) => {

"use strict";
module.exports = require("next/dist/server/app-render/after-task-async-storage.external.js");

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

/***/ 13370:
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
const module4 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 36054));
const page5 = () => Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 76525));


// We inject the tree and pages here so that we can use them in the route
// module.
const tree = {
        children: [
        '',
        {
        children: [
        'logs',
        {
        children: ['__PAGE__', {}, {
          page: [page5, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/logs/page.tsx"],
          
        }]
      },
        {
        'layout': [module4, "/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/logs/layout.tsx"],
        
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
const pages = ["/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/logs/page.tsx"];


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
        page: "/logs/page",
        pathname: "/logs",
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

/***/ 27910:
/***/ ((module) => {

"use strict";
module.exports = require("stream");

/***/ }),

/***/ 28086:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._1vjdqev0 {\n  position: relative;\n  display: inline-block;\n}\n._1vjdqev1 {\n  display: flex;\n  align-items: center;\n  gap: 0.5rem;\n  padding: 0.5rem 0.75rem;\n  background-color: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  color: var(--_19ez2rz0);\n  border-radius: 4px;\n  cursor: pointer;\n  font-size: 0.9rem;\n  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;\n  transition: border-color 0.2s, background-color 0.2s;\n}\n._1vjdqev1:hover {\n  background-color: var(--_19ez2rz8);\n  border-color: var(--_19ez2rzk);\n}\n._1vjdqev1:focus {\n  outline: none;\n  border-color: var(--_19ez2rzl);\n  box-shadow: 0 0 0 2px var(--_19ez2rz17);\n}\n._1vjdqev2 {\n  font-size: 1rem;\n}\n._1vjdqev3 {\n  flex: 1 1;\n  text-align: left;\n  min-width: 120px;\n}\n._1vjdqev4 {\n  font-size: 0.7rem;\n  color: var(--_19ez2rz2);\n}\n._1vjdqev5 {\n  position: absolute;\n  top: 100%;\n  left: 0;\n  margin-top: 4px;\n  background-color: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: 6px;\n  box-shadow: var(--_19ez2rz3l);\n  z-index: 100;\n  min-width: 200px;\n  overflow: hidden;\n}\n._1vjdqev6 {\n  display: flex;\n  flex-direction: column;\n  padding: 0.5rem;\n}\n._1vjdqev7 {\n  padding: 0.5rem 0.75rem;\n  background: none;\n  border: none;\n  color: var(--_19ez2rz0);\n  text-align: left;\n  cursor: pointer;\n  border-radius: 4px;\n  font-size: 0.85rem;\n  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;\n  transition: background-color 0.15s;\n}\n._1vjdqev7:hover {\n  background-color: var(--_19ez2rz8);\n}\n._1vjdqev7:focus {\n  outline: none;\n  box-shadow: inset 0 0 0 2px var(--_19ez2rz18);\n}\n._1vjdqev8 {\n  background-color: var(--_19ez2rzm);\n  color: var(--_19ez2rzq);\n}\n._1vjdqev9 {\n  height: 1px;\n  background-color: var(--_19ez2rze);\n  margin: 0.25rem 0;\n}\n._1vjdqeva {\n  width: 100%;\n  padding: 0.5rem 0.75rem;\n  background: none;\n  border: none;\n  color: var(--_19ez2rzm);\n  text-align: left;\n  cursor: pointer;\n  font-size: 0.85rem;\n  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;\n  transition: background-color 0.15s;\n}\n._1vjdqeva:hover {\n  background-color: var(--_19ez2rz8);\n}\n._1vjdqevb {\n  padding: 0.75rem;\n}\n._1vjdqevc {\n  display: flex;\n  align-items: center;\n  gap: 0.5rem;\n  margin-bottom: 0.75rem;\n  font-size: 0.85rem;\n  color: var(--_19ez2rz2);\n}\n._1vjdqevd {\n  background: none;\n  border: none;\n  color: var(--_19ez2rzm);\n  cursor: pointer;\n  font-size: 0.85rem;\n  padding: 0;\n  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;\n}\n._1vjdqevd:hover {\n  text-decoration: underline;\n}\n._1vjdqeve {\n  display: flex;\n  flex-direction: column;\n  gap: 0.5rem;\n  margin-bottom: 0.75rem;\n}\n._1vjdqevf {\n  display: flex;\n  flex-direction: column;\n  gap: 0.25rem;\n}\n._1vjdqevf span {\n  font-size: 0.75rem;\n  color: var(--_19ez2rz2);\n}\n._1vjdqevg {\n  padding: 0.5rem;\n  background-color: var(--_19ez2rz6);\n  border: 1px solid var(--_19ez2rzk);\n  color: var(--_19ez2rz0);\n  border-radius: 4px;\n  font-size: 0.85rem;\n  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;\n}\n._1vjdqevg:focus {\n  outline: none;\n  border-color: var(--_19ez2rzl);\n}\n._1vjdqevh {\n  width: 100%;\n  padding: 0.5rem;\n  background-color: var(--_19ez2rzm);\n  border: none;\n  color: var(--_19ez2rzq);\n  border-radius: 4px;\n  cursor: pointer;\n  font-size: 0.85rem;\n  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;\n  transition: background-color 0.2s;\n}\n._1vjdqevh:hover:not(:disabled) {\n  background-color: var(--_19ez2rzn);\n}\n._1vjdqevh:disabled {\n  background-color: var(--_19ez2rze);\n  color: var(--_19ez2rz2);\n  cursor: not-allowed;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


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

/***/ 36054:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": () => (/* binding */ LogsLayout),
/* harmony export */   metadata: () => (/* binding */ metadata)
/* harmony export */ });
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(12129);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__);

const metadata = {
    title: "Logs - Stapler Squad",
    description: "View and search application logs from your Stapler Squad sessions."
};
function LogsLayout({ children }) {
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.Fragment, {
        children: children
    });
}


/***/ }),

/***/ 37842:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._1g1k1fa0 {\n  flex: 1 1;\n  overflow: auto;\n  min-height: 0;\n  background-color: var(--_19ez2rz6);\n  border-radius: var(--_19ez2rz37);\n  border: 1px solid var(--_19ez2rze);\n}\n._1g1k1fa1 {\n  border-bottom: 1px solid var(--_19ez2rz7);\n}\n._1g1k1fa2 {\n  display: flex;\n  align-items: center;\n  gap: 0.75rem;\n  padding: 0.6rem 0.75rem;\n  cursor: pointer;\n  font-size: var(--_19ez2rz3a);\n  font-family: var(--_19ez2rz2u);\n}\n._1g1k1fa2:hover {\n  background-color: var(--_19ez2rz7);\n}\n._1g1k1fa3 {\n  width: 1rem;\n  flex-shrink: 0;\n  color: var(--_19ez2rz2);\n  transition: transform 0.1s;\n}\n._1g1k1fa4 {\n  transform: rotate(90deg);\n}\n._1g1k1fa5 {\n  flex-shrink: 0;\n  min-width: 3.5rem;\n  text-align: right;\n  padding: 0.1rem 0.4rem;\n  border-radius: var(--_19ez2rz36);\n  background-color: var(--_19ez2rz7);\n  color: var(--_19ez2rz0);\n  font-weight: 600;\n}\n._1g1k1fa6 {\n  flex-shrink: 0;\n  min-width: 4.5rem;\n  font-weight: 600;\n  text-transform: uppercase;\n}\n._1g1k1fa7 {\n  color: var(--_19ez2rz2);\n}\n._1g1k1fa8 {\n  color: var(--_19ez2rzm);\n}\n._1g1k1fa9 {\n  color: var(--_19ez2rzx);\n}\n._1g1k1faa {\n  color: var(--_19ez2rz10);\n}\n._1g1k1fab {\n  flex: 1 1;\n  white-space: pre-wrap;\n  word-break: break-word;\n  color: var(--_19ez2rz0);\n}\n._1g1k1fac {\n  color: var(--_19ez2rzm);\n  font-weight: 500;\n}\n._1g1k1fad {\n  padding: 0 0.75rem 0.75rem 2.75rem;\n  display: flex;\n  flex-direction: column;\n  gap: 0.35rem;\n}\n._1g1k1fae {\n  display: flex;\n  gap: 0.75rem;\n  font-size: var(--_19ez2rz3a);\n  font-family: var(--_19ez2rz2u);\n  color: var(--_19ez2rz2);\n}\n._1g1k1faf {\n  flex-shrink: 0;\n  white-space: nowrap;\n}\n._1g1k1fag {\n  color: var(--_19ez2rz0);\n  white-space: pre-wrap;\n  word-break: break-word;\n}\n._1g1k1fah {\n  color: var(--_19ez2rz2);\n  font-style: italic;\n}\n._1g1k1fai {\n  padding: 3rem 2rem;\n  text-align: center;\n  color: var(--_19ez2rz2);\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 50565:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".tq02c00 {\n  display: flex;\n  flex-wrap: wrap;\n  gap: 0.5rem;\n  align-items: center;\n  margin-bottom: 1rem;\n}\n.tq02c01 {\n  display: inline-flex;\n  align-items: center;\n  gap: 0.25rem;\n  padding: 0.25rem 0.5rem;\n  background-color: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rzk);\n  border-radius: 16px;\n  font-size: 0.8rem;\n  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;\n}\n.tq02c02 {\n  color: var(--_19ez2rz2);\n}\n.tq02c03 {\n  color: var(--_19ez2rz0);\n  font-weight: 500;\n}\n.tq02c04 {\n  display: flex;\n  align-items: center;\n  justify-content: center;\n  width: 16px;\n  height: 16px;\n  margin-left: 0.25rem;\n  padding: 0;\n  background: none;\n  border: none;\n  color: var(--_19ez2rz2);\n  cursor: pointer;\n  border-radius: 50%;\n  font-size: 1rem;\n  line-height: 1;\n  transition: color 0.15s, background-color 0.15s;\n}\n.tq02c04:hover {\n  color: var(--_19ez2rz10);\n  background-color: var(--_19ez2rz11);\n}\n.tq02c04:focus {\n  outline: none;\n  box-shadow: 0 0 0 2px var(--_19ez2rz18);\n}\n.tq02c05 {\n  padding: 0.25rem 0.5rem;\n  background: none;\n  border: none;\n  color: var(--_19ez2rzm);\n  cursor: pointer;\n  font-size: 0.8rem;\n  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;\n  transition: color 0.15s;\n}\n.tq02c05:hover {\n  color: var(--_19ez2rzn);\n  text-decoration: underline;\n}\n.tq02c05:focus {\n  outline: none;\n  text-decoration: underline;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 51109:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 76525));


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

/***/ 67397:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  "default": () => (/* binding */ LogsPage)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
var react_default = /*#__PURE__*/__webpack_require__.n(react);
// EXTERNAL MODULE: ./src/lib/utils/datetime.ts
var datetime = __webpack_require__(43795);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Flogs%2FTimeRangePicker.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VXwVLbMBC95yt06UBmcEY2BIP4Bo49M4q1idXIWiPJIaTDv3ckO2AbA05C28khsax9Wr19b6XMHuLNL_EIG0p-Twgp0UonUTNiQHEnN3A3IURIWyr-zIjUSmqIFgqz9d3kZTLbR8ch-nXeUsHWB3IlVzqSDgrLSAbagfHDK14yQmdzA4V_LLkQUq_2Q4TO0v2rBc_WK4OVFlGGCg0jG27Oo-ghvoVdYnbpNMxCI8AwEpdbYlFJ0ZsFYdYgAG0BRIYLWVlGrsqQfVYZ6yNKlPvMl6hdZOUOfLK3TZJhcMkLqZ4ZObtHzTM8uyBn96BV-PFzUWlXkXvU_rFAjbbkWaDWGa73lDdZhDwJnSX24t3-w3CXeZbjBkzg_yu2btqbHZyxnvbAl5hVNoBj5XzxGdGo4Ssc1ay0jWzOBT4xQsMnKbe9mXHaXTIJi7V4jgPLrRmX9QwFW0biwCFsXRSkxoiCpfNjhdTRkxQuZyROqK9nC-GqvwadpU0tB3eTdFOc96zCFxZV5ep6YslITOkP_-CTYYSGfLhZSR2F1428vlPbPQFfNyu0-O9GXdYF2kVSi8AjpT3WEkprEK-upfIQuRQCdIeK62Hf--9ISANZzVCGqir0gNc7YGnN65hu0Ffh2_MnLh_SyYDHh5tBRy038--2_nufx_Ou0dMDjd4J_dTGbyKR2oL72Ko92JtRuRQfN9_HLt5twMtBrnIXBD_GJbX-a3v50iS1aDrAPADv20Fjzr-hs-IQnf13RfHjFbXoWzV95-bslCtB0y4X6BwWrF2bYdbGtG3R2-hRtR1dxzduTqtqewOtegWNCcjQ8LrelRZgvLM7IXBofx5Zh9YSyyOXSAaQbMn1wNl8SJVXQ4fImEZyPeq4XR93lfxur7c3fModrYWTj2iSY3gspqMt9XjC5ftfNMzeVTuv_cc0unMmpOULBWI6qn3qHtev4aOiP_n_knS6kkZ_8Ch8AnE3efkDCr8xw9wNAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Flogs_2FTimeRangePicker_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VXwVLbMBC95yt06UBmcEY2BIP4Bo49M4q1idXIWiPJIaTDv3ckO2AbA05C28khsax9Wr19b6XMHuLNL_EIG0p_Twgp0UonUTNiQHEnN3A3IURIWyr_zIjUSmqIFgqz9d3kZTLbR8ch_nXeUsHWB3IlVzqSDgrLSAbagfHDK14yQmdzA4V_LLkQUq_2Q4TO0v2rBc_WK4OVFlGGCg0jG27Oo_ghvoVdYnbpNMxCI8AwEpdbYlFJ0ZsFYdYgAG0BRIYLWVlGrsqQfVYZ6yNKlPvMl6hdZOUOfLK3TZJhcMkLqZ4ZObtHzTM8uyBn96BV_PFzUWlXkXvU_rFAjbbkWaDWGa73lDdZhDwJnSX24t3_w3CXeZbjBkzg_yu2btqbHZyxnvbAl5hVNoBj5XzxGdGo4Ssc1ay0jWzOBT4xQsMnKbe9mXHaXTIJi7V4jgPLrRmX9QwFW0biwCFsXRSkxoiCpfNjhdTRkxQuZyROqK9nC_GqvwadpU0tB3eTdFOc96zCFxZV5ep6YslITOkP_CTYYSGfLhZSR2F1428vlPbPQFfNyu0_O9GXdYF2kVSi8AjpT3WEkprEK_upfIQuRQCdIeK62Hf_9ISANZzVCGqir0gNc7YGnN65hu0Ffh2_MnLh_SyYDHh5tBRy038_2_nufx_Ou0dMDjd4J_dTGbyKR2oL72Ko92JtRuRQfN9_HLt5twMtBrnIXBD_GJbX_a3v50iS1aDrAPADv20Fjzr_hs_IQnf13RfHjFbXoWzV95_bslCtB0y4X6BwWrF2bYdbGtG3R2_hRtR1dxzduTqtqewOtegWNCcjQ8LrelRZgvLM7IXBofx5Zh9YSyyOXSAaQbMn1wNl8SJVXQ4fImEZyPeq4XR93lfxur7c3fModrYWTj2iSY3gspqMt9XjC5ftfNMzeVTuv_cc0unMmpOULBWI6qn3qHtev4aOiP_n_knS6kkZ_8Ch8AnE3efkDCr8xw9wNAAA_22_7D = __webpack_require__(28086);
;// ./src/components/logs/TimeRangePicker.css.ts


var applyButton = '_1vjdqevh';
var backButton = '_1vjdqevd';
var chevron = '_1vjdqev4';
var container = '_1vjdqev0';
var customButton = '_1vjdqeva';
var customHeader = '_1vjdqevc';
var customInputs = '_1vjdqeve';
var customRange = '_1vjdqevb';
var dateInput = '_1vjdqevg';
var divider = '_1vjdqev9';
var dropdown = '_1vjdqev5';
var icon = '_1vjdqev2';
var inputGroup = '_1vjdqevf';
var label = '_1vjdqev3';
var presetButton = '_1vjdqev7';
var presetButtonActive = '_1vjdqev8';
var presets = '_1vjdqev6';
var trigger = '_1vjdqev1';
;// ./src/components/logs/TimeRangePicker.tsx
/* __next_internal_client_entry_do_not_use__ TimeRangePicker auto */ 



function TimeRangePicker({ value, onChange, className }) {
    const [isOpen, setIsOpen] = (0,react.useState)(false);
    const [showCustom, setShowCustom] = (0,react.useState)(false);
    const [customStart, setCustomStart] = (0,react.useState)('');
    const [customEnd, setCustomEnd] = (0,react.useState)('');
    const dropdownRef = (0,react.useRef)(null);
    // Get current display label
    const getDisplayLabel = ()=>{
        if (value.preset) {
            const preset = datetime/* TIME_RANGE_PRESETS */.jX.find((p)=>p.value === value.preset);
            return preset?.label || 'Custom';
        }
        return 'Custom range';
    };
    // Handle preset selection
    const handlePresetClick = (preset)=>{
        const range = preset.getRange();
        onChange({
            start: range.start,
            end: range.end,
            preset: preset.value
        });
        setIsOpen(false);
        setShowCustom(false);
    };
    // Handle custom range submission
    const handleCustomSubmit = ()=>{
        if (customStart && customEnd) {
            const start = new Date(customStart);
            const end = new Date(customEnd);
            if (!isNaN(start.getTime()) && !isNaN(end.getTime()) && start < end) {
                onChange({
                    start,
                    end,
                    preset: undefined
                });
                setIsOpen(false);
                setShowCustom(false);
            }
        }
    };
    // Close dropdown when clicking outside
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const handleClickOutside = (event)=>{
            if (dropdownRef.current && !dropdownRef.current.contains(event.target)) {
                setIsOpen(false);
                setShowCustom(false);
            }
        };
        if (isOpen) {
            document.addEventListener('mousedown', handleClickOutside);
        }
        return ()=>{
            document.removeEventListener('mousedown', handleClickOutside);
        };
    }, [
        isOpen
    ]);
    // Handle keyboard navigation
    const handleKeyDown = (event)=>{
        if (event.key === 'Escape') {
            setIsOpen(false);
            setShowCustom(false);
        } else if (event.key === 'Enter' && !isOpen) {
            setIsOpen(true);
        }
    };
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: `${container} ${className || ''}`,
        ref: dropdownRef,
        onKeyDown: handleKeyDown,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                className: trigger,
                onClick: ()=>setIsOpen(!isOpen),
                "aria-haspopup": "listbox",
                "aria-expanded": isOpen,
                "aria-label": `Time range: ${getDisplayLabel()}`,
                type: "button",
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        className: icon,
                        children: "\uD83D\uDD50"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        className: label,
                        children: getDisplayLabel()
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        className: chevron,
                        children: isOpen ? '▲' : '▼'
                    })
                ]
            }),
            isOpen && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: dropdown,
                role: "listbox",
                "aria-label": "Select time range",
                children: !showCustom ? /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                            className: presets,
                            children: datetime/* TIME_RANGE_PRESETS */.jX.map((preset)=>/*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                    className: `${presetButton} ${value.preset === preset.value ? presetButtonActive : ''}`,
                                    onClick: ()=>handlePresetClick(preset),
                                    role: "option",
                                    "aria-selected": value.preset === preset.value,
                                    children: preset.label
                                }, preset.value))
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                            className: divider
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            className: customButton,
                            onClick: ()=>setShowCustom(true),
                            type: "button",
                            children: "Custom range..."
                        })
                    ]
                }) : /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                    className: customRange,
                    children: [
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                            className: customHeader,
                            children: [
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                    className: backButton,
                                    onClick: ()=>setShowCustom(false),
                                    type: "button",
                                    "aria-label": "Back to presets",
                                    children: "← Back"
                                }),
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                    children: "Custom Range"
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                            className: customInputs,
                            children: [
                                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                                    className: inputGroup,
                                    children: [
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                            children: "Start:"
                                        }),
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                            type: "datetime-local",
                                            value: customStart,
                                            onChange: (e)=>setCustomStart(e.target.value),
                                            className: dateInput,
                                            "aria-label": "Start date and time"
                                        })
                                    ]
                                }),
                                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                                    className: inputGroup,
                                    children: [
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                            children: "End:"
                                        }),
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                            type: "datetime-local",
                                            value: customEnd,
                                            onChange: (e)=>setCustomEnd(e.target.value),
                                            className: dateInput,
                                            "aria-label": "End date and time"
                                        })
                                    ]
                                })
                            ]
                        }),
                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                            className: applyButton,
                            onClick: handleCustomSubmit,
                            disabled: !customStart || !customEnd,
                            type: "button",
                            children: "Apply"
                        })
                    ]
                })
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Flogs%2FFilterPill.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61T0Y6bMBB8z1fsS5VGOkeGNr2r-w157HPlYAPuwS61zYWk6r9XNpA66XHXSickYNfLzDBjb_0Pnhecw88VgDKua-RJQNno4csK4pMdrewEhHtoVaHg253VbShlYypkxuvWCSg0em1Du5W2MsgO5D21ArI4_Wu1HdmyazaDjUHNZtIFyIk4n5k7qZTB6tJLRB1k8VhZ6lGxghqyAp6kfc_Yt-yzPuf2fL-JU2SVtgKybgBHjVE3U4_JFLNSmd4JyD51ozOEnjlz1oH_YeKNzVK2pjkJWO8JZUHrO1jvNTbx5euhR9_DnjCULSG5ThY6sSaP1jyrOt8kcx-W5_jmouWoTVV7ATvOk28_Ph_2gu_fe-dNeWIFodfo06WjUb7-40k9sc31tAkaXfqF5K7DEoCEOo1mrhf9ACh668JaR2aWdRPZjr-7SSybhMRtd1EdOt5KdMYbQjGSAt9mO3f315Ya-6mpoqYnbZdjycZcXtubWba5Qi2p6F1Epd4HwalLA3O1VHQUwOOVd8Mt3EMKt4tA_3Ry_jOMdimMNz4oixGlf_laFBjFej14pnRBVo5oPSptg8VXWC8E8CLCb7r1oOlaBQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Flogs_2FFilterPill_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61T0Y6bMBB8z1fsS5VGOkeGNr2r_w157HPlYAPuwS61zYWk6r9XNpA66XHXSickYNfLzDBjb_0Pnhecw88VgDKua_RJQNno4csK4pMdrewEhHtoVaHg253VbShlYypkxuvWCSg0em1Du5W2MsgO5D21ArI4_Wu1HdmyazaDjUHNZtIFyIk4n5k7qZTB6tJLRB1k8VhZ6lGxghqyAp6kfc_Yt_yzPuf2fL_JU2SVtgKybgBHjVE3U4_JFLNSmd4JyD51ozOEnjlz1oH_YeKNzVK2pjkJWO8JZUHrO1jvNTbx5euhR9_DnjCULSG5ThY6sSaP1jyrOt8kcx_W5_jmouWoTVV7ATvOk28_Ph_2gu_fe_dNeWIFodfo06WjUb7_40k9sc31tAkaXfqF5K7DEoCEOo1mrhf9ACh668JaR2aWdRPZjr_7SSybhMRtd1EdOt5KdMYbQjGSAt9mO3f315Ya_6mpoqYnbZdjycZcXtubWba5Qi2p6F1Epd4HwalLA3O1VHQUwOOVd8Mt3EMKt4tA_3Ry_jOMdimMNz4oixGlf_laFBjFej14pnRBVo5oPSptg8VXWC8E8CLCb7r1oOlaBQAA_22_7D = __webpack_require__(50565);
;// ./src/components/logs/FilterPill.css.ts


var clearAllButton = 'tq02c05';
var FilterPill_css_container = 'tq02c00';
var FilterPill_css_label = 'tq02c02';
var pill = 'tq02c01';
var removeButton = 'tq02c04';
var FilterPill_css_value = 'tq02c03';
;// ./src/components/logs/FilterPill.tsx
/* __next_internal_client_entry_do_not_use__ FilterPill,FilterPills auto */ 


function FilterPill({ label, value, onRemove, color, className }) {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: `${pill} ${className || ''}`,
        style: color ? {
            borderColor: color
        } : undefined,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                className: FilterPill_css_label,
                children: [
                    label,
                    ":"
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                className: FilterPill_css_value,
                style: color ? {
                    color
                } : undefined,
                children: value
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                className: removeButton,
                onClick: onRemove,
                "aria-label": `Remove ${label}: ${value} filter`,
                type: "button",
                children: "\xd7"
            })
        ]
    });
}
function FilterPills({ children, onClearAll, className }) {
    const hasChildren = react_default().Children.count(children) > 0;
    if (!hasChildren) {
        return null;
    }
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: `${FilterPill_css_container} ${className || ''}`,
        children: [
            children,
            onClearAll && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                className: clearAllButton,
                onClick: onClearAll,
                type: "button",
                "aria-label": "Clear all filters",
                children: "Clear all"
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/timestamp.js
var timestamp = __webpack_require__(42865);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Flogs%2FExportButton.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71Ty27bMBC8-yv2UjgGKkGSn6G_wceeA1pcS4QprkBSfgX594C0lEqq0-YQ9CZSu7PDmdn4JV1kVXW0CbxOAGqy0knSDAwq7uQJt5O3SdwVpaFISFsrfmVwUHjZTgC4koWOpMPKMshROzT-uuA1gyReGqz8seZCSF10V5DE6-7XnufHwlCjRZSTIsPgxM1TFL2kz3jLzG09C1VkBBoGaX0BS0qKUdUxVD0ESHoAkeFCNpbBog7s88ZY31GT7JgfSLvIyht6spuOZbg98EqqK4PpjjTPafoTpjvUKnz82jfaNbAj7Y8VabI1z9H3OsN1Jy1XCpI4s0NpWUknNEyTe2JCWr5XKGZB73-ps-k_7mFFNhvN6gYEfKp5Lt01GNMXRJOLuFJ0RjFoz0ZR4XtLqnH3d1LNIE2SH_5gZFE6Bon_rrgppI7C_1b473EdH1m7aifQJbIlF3QeY89VaLtFUgu8BMqBpdTRWQpXMkg3yR3E23JQHqKUQqAeaDH_po1YZZ_vhHdCY1-L7vyXrP-XVHdStX47vPjEyEIzUHhw4-CPDYckTpfDNZjf1-DLue-1LkLT4JHP7RtbnlnraJ9nZ04PaPkb6Iz3CC99PHolqz9nfRj36QZ-LIHXhgFvHA1A1wG0bEemX1sS7AF7Gm2OtpO3d-grlqXbBQAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Flogs_2FExportButton_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71Ty27bMBC8_yv2UjgGKkGSn6G_wceeA1pcS4QprkBSfgX594C0lEqq0_YQ9CZSu7PDmdn4JV1kVXW0CbxOAGqy0knSDAwq7uQJt5O3SdwVpaFISFsrfmVwUHjZTgC4koWOpMPKMshROzT_uuA1gyReGqz8seZCSF10V5DE6_7XnufHwlCjRZSTIsPgxM1TFL2kz3jLzG09C1VkBBoGaX0BS0qKUdUxVD0ESHoAkeFCNpbBog7s88ZY31GT7JgfSLvIyht6spuOZbg98EqqK4PpjjTPafoTpjvUKnz82jfaNbAj7Y8VabI1z9H3OsN1Jy1XCpI4s0NpWUknNEyTe2JCWr5XKGZB73_ps_k_7mFFNhvN6gYEfKp5Lt01GNMXRJOLuFJ0RjFoz0ZR4XtLqnH3d1LNIE2SH_5gZFE6Bon_rrgppI7C_1b473EdH1m7aifQJbIlF3QeY89VaLtFUgu8BMqBpdTRWQpXMkg3yR3E23JQHqKUQqAeaDH_po1YZZ_vhHdCY1_L7vyXrP_XVHdStX47vPjEyEIzUHhw4_CPDYckTpfDNZjf1_DLue_1LkLT4JHP7RtbnlnraJ9nZ04PaPkb6Iz3CC99PHolqz9nfRj36QZ_LIHXhgFvHA1A1wG0bEemX1sS7AF7Gm2OtpO3d_grlqXbBQAA_22_7D = __webpack_require__(85021);
;// ./src/components/logs/ExportButton.css.ts


var ExportButton_css_button = '_142mmks1';
var ExportButton_css_container = '_142mmks0';
var ExportButton_css_divider = '_142mmks7';
var ExportButton_css_dropdown = '_142mmks2';
var ExportButton_css_icon = '_142mmks4';
var ExportButton_css_option = '_142mmks3';
var optionDesc = '_142mmks6';
var optionLabel = '_142mmks5';
;// ./src/components/logs/ExportButton.tsx
/* __next_internal_client_entry_do_not_use__ ExportButton auto */ 



function ExportButton({ logs, disabled, className }) {
    const [showDropdown, setShowDropdown] = (0,react.useState)(false);
    const [exporting, setExporting] = (0,react.useState)(false);
    const containerRef = (0,react.useRef)(null);
    // Close dropdown when clicking outside
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const handleClickOutside = (event)=>{
            if (containerRef.current && !containerRef.current.contains(event.target)) {
                setShowDropdown(false);
            }
        };
        if (showDropdown) {
            document.addEventListener('mousedown', handleClickOutside);
        }
        return ()=>{
            document.removeEventListener('mousedown', handleClickOutside);
        };
    }, [
        showDropdown
    ]);
    // Format log entry for export
    const formatLogEntry = (log)=>({
            timestamp: log.timestamp ? (0,timestamp/* timestampDate */.FP)(log.timestamp).toISOString() : '',
            level: log.level,
            source: log.source || '',
            message: log.message
        });
    // Export as JSON
    const exportJSON = ()=>{
        const data = logs.map(formatLogEntry);
        const blob = new Blob([
            JSON.stringify(data, null, 2)
        ], {
            type: 'application/json'
        });
        downloadBlob(blob, `logs-${getTimestamp()}.json`);
    };
    // Export as CSV
    const exportCSV = ()=>{
        const headers = [
            'timestamp',
            'level',
            'source',
            'message'
        ];
        const rows = logs.map((log)=>{
            const entry = formatLogEntry(log);
            return [
                entry.timestamp,
                entry.level,
                entry.source,
                // Escape quotes and wrap in quotes if contains comma or newline
                `"${entry.message.replace(/"/g, '""')}"`
            ].join(',');
        });
        const csv = [
            headers.join(','),
            ...rows
        ].join('\n');
        const blob = new Blob([
            csv
        ], {
            type: 'text/csv'
        });
        downloadBlob(blob, `logs-${getTimestamp()}.csv`);
    };
    // Export as plain text
    const exportTXT = ()=>{
        const lines = logs.map((log)=>{
            const entry = formatLogEntry(log);
            return `[${entry.timestamp}] [${entry.level}] [${entry.source}] ${entry.message}`;
        });
        const text = lines.join('\n');
        const blob = new Blob([
            text
        ], {
            type: 'text/plain'
        });
        downloadBlob(blob, `logs-${getTimestamp()}.txt`);
    };
    // Generate timestamp for filename
    const getTimestamp = ()=>{
        return new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
    };
    // Download blob as file
    const downloadBlob = (blob, filename)=>{
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
    };
    // Handle export
    const handleExport = async (format)=>{
        setExporting(true);
        setShowDropdown(false);
        try {
            switch(format){
                case 'json':
                    exportJSON();
                    break;
                case 'csv':
                    exportCSV();
                    break;
                case 'txt':
                    exportTXT();
                    break;
            }
        } finally{
            setExporting(false);
        }
    };
    // Copy to clipboard
    const copyToClipboard = async ()=>{
        setShowDropdown(false);
        const lines = logs.map((log)=>{
            const entry = formatLogEntry(log);
            return `[${entry.timestamp}] [${entry.level}] [${entry.source}] ${entry.message}`;
        });
        await navigator.clipboard.writeText(lines.join('\n'));
    };
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: `${ExportButton_css_container} ${className || ''}`,
        ref: containerRef,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                className: ExportButton_css_button,
                onClick: ()=>setShowDropdown(!showDropdown),
                disabled: disabled || logs.length === 0 || exporting,
                "aria-haspopup": "menu",
                "aria-expanded": showDropdown,
                "aria-label": `Export ${logs.length} logs`,
                title: logs.length === 0 ? 'No logs to export' : `Export ${logs.length} logs`,
                children: [
                    exporting ? '...' : '↓',
                    " Export"
                ]
            }),
            showDropdown && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: ExportButton_css_dropdown,
                role: "menu",
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                        className: ExportButton_css_option,
                        onClick: ()=>handleExport('json'),
                        role: "menuitem",
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: ExportButton_css_icon
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: optionLabel,
                                children: "JSON"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: optionDesc,
                                children: "Structured data"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                        className: ExportButton_css_option,
                        onClick: ()=>handleExport('csv'),
                        role: "menuitem",
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: ExportButton_css_icon,
                                children: "\uD83D\uDCCA"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: optionLabel,
                                children: "CSV"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: optionDesc,
                                children: "Spreadsheet"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                        className: ExportButton_css_option,
                        onClick: ()=>handleExport('txt'),
                        role: "menuitem",
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: ExportButton_css_icon,
                                children: "\uD83D\uDCC4"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: optionLabel,
                                children: "Plain Text"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: optionDesc,
                                children: "Simple format"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: ExportButton_css_divider
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                        className: ExportButton_css_option,
                        onClick: copyToClipboard,
                        role: "menuitem",
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: ExportButton_css_icon,
                                children: "\uD83D\uDCCB"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: optionLabel,
                                children: "Copy All"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: optionDesc,
                                children: "To clipboard"
                            })
                        ]
                    })
                ]
            })
        ]
    });
}

;// ./src/lib/logs/logPatterns.ts
// logPatterns.ts — Datadog-style "log pattern" clustering: groups log
// entries by message with dynamic content (UUIDs, paths, embedded objects,
// numbers) normalized into placeholders, so structurally identical messages
// collapse into one row even when a value is interpolated into the message
// text.
const UUID_RE = /[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}/g;
const PATH_RE = /(\/[A-Za-z0-9_.-]+){2,}/g;
const OBJ_RE = /\{[^{}]*\}/g;
const NUMBER_RE = /[0-9]+/g;
/** Normalizes one log message into a pattern with placeholders substituted for dynamic content. */ function normalizeMessage(message) {
    return message.replace(UUID_RE, "<uuid>").replace(PATH_RE, "<path>").replace(OBJ_RE, "<obj>").replace(NUMBER_RE, "<n>");
}
/**
 * Groups log entries into patterns by (level, normalized message), sorted by
 * count descending, so a page full of near-identical lines collapses into
 * one row with an expandable list of the actual matching entries (which
 * still show whatever varied — the path, the UUID, the count — since
 * normalization only affects the grouping key, not what's displayed on
 * expand).
 */ function groupLogsByPattern(entries) {
    const groups = new Map();
    for (const entry of entries){
        const pattern = normalizeMessage(entry.message);
        const key = `${entry.level} ${pattern}`;
        const existing = groups.get(key);
        if (existing) {
            existing.count += 1;
            existing.entries.push(entry);
        } else {
            groups.set(key, {
                key,
                level: entry.level,
                pattern,
                count: 1,
                entries: [
                    entry
                ]
            });
        }
    }
    return Array.from(groups.values()).sort((a, b)=>b.count - a.count);
}

// EXTERNAL MODULE: ./src/lib/analytics/index.ts
var analytics = __webpack_require__(54953);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Flogs%2FPatternsView.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA6VUy3LjIBC8-ys4xgdcQn6tyceksBhJlCVQDSh-bOXfUyDZFrLlZGtPsoceZrp7mMUHK9iB5SIhf2eE5BWcOGHvM0LMJ2BemSMnonXGR2qlaQmqKB0niQ_sRXYo0LRa0sxUBjn5FPhG6QfbwSXFy2YeUAYlIEUhVWvHkOV2gOGENSdiTaXkCAbz99nXbHFtloVm-4v3xjlTT-du49w05Eplm0qceWDsOxCVKjRVDmrLSQbaAfpwIRpOksV2jVD7_42QUunCxzYI9fAoa9F6DRqjrtm50Y5adYEH2mJ-O89FrarzGJG2o7Z56R3piP-g-4jxMiQdlXQlJ6zv1vOmtkSlD72bT69KQ58OhbbKKaN59zs36LkzGxVahUI3ACdonHDwtkskFHFP69u4xU34Ges7XS6uyjo4ORoM4gT9_I2cYJ0Tqx7-euD6ofyFhBOSJHfrjv1r2CRJxG7zM7vVjd3jTT3jgZBt0wBmwkJUZhvKTPo2QP6ZRtYxcjeNPMVIMY1kSQzdj5fLsVQOqG1EBpw0CPSIogkHBiXdI4gDJ-FDfeSlF4M62WuaI63XI9dkyL7P1fVx377p_bE_7I9gtlQIWfdMMlO1tR6skGWXOqgHzzfReOX87xL53YTkEyMbOaVN59Mgr5iWvHso_2z14PLy9YBftXHnCjhRTlQqi_JV7OgymPhkqdz3_bRU30rZY0cqBwAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Flogs_2FPatternsView_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA6VUy3LjIBC8_ys4xgdcQn6tyceksBhJlCVQDSh_bOXfUyDZFrLlZGtPsoceZrp7mMUHK9iB5SIhf2eE5BWcOGHvM0LMJ2BemSMnonXGR2qlaQmqKB0niQ_sRXYo0LRa0sxUBjn5FPhG6QfbwSXFy2YeUAYlIEUhVWvHkOV2gOGENSdiTaXkCAbz99nXbHFtloVm_4v3xjlTT_du49w05Eplm0qceWDsOxCVKjRVDmrLSQbaAfpwIRpOksV2jVD7_42QUunCxzYI9fAoa9F6DRqjrtm50Y5adYEH2mJ_O89FrarzGJG2o7Z56R3piP_g_4jxMiQdlXQlJ6zv1vOmtkSlD72bT69KQ58OhbbKKaN59zs36LkzGxVahUI3ACdonHDwtkskFHFP69u4xU34Ges7XS6uyjo4ORoM4gT9_I2cYJ0Tqx7_euD6ofyFhBOSJHfrjv1r2CRJxG7zM7vVjd3jTT3jgZBt0wBmwkJUZhvKTPo2QP6ZRtYxcjeNPMVIMY1kSQzdj5fLsVQOqG1EBpw0CPSIogkHBiXdI4gDJ_FDfeSlF4M62WuaI63XI9dkyL7P1fVx377p_bE_7I9gtlQIWfdMMlO1tR6skGWXOqgHzzfReOX87xL53YTkEyMbOaVN59Mgr5iWvHso_2z14PLy9YBftXHnCjhRTlQqi_JV7OgymPhkqdz3_bRU30rZY0cqBwAA_22_7D = __webpack_require__(37842);
;// ./src/components/logs/PatternsView.css.ts


var PatternsView_css_container = '_1g1k1fa0';
var count = '_1g1k1fa5';
var disclosure = '_1g1k1fa3';
var disclosureOpen = '_1g1k1fa4';
var empty = '_1g1k1fai';
var example = '_1g1k1fae';
var exampleMessage = '_1g1k1fag';
var exampleTimestamp = '_1g1k1faf';
var examples = '_1g1k1fad';
var examplesMore = '_1g1k1fah';
var level = '_1g1k1fa6';
var levelDebug = '_1g1k1fa7';
var levelError = '_1g1k1faa';
var levelInfo = '_1g1k1fa8';
var levelWarning = '_1g1k1fa9';
var pattern = '_1g1k1fab';
var placeholder = '_1g1k1fac';
var row = '_1g1k1fa1';
var rowHeader = '_1g1k1fa2';
;// ./src/components/logs/PatternsView.tsx
/* __next_internal_client_entry_do_not_use__ PatternsView auto */ 





const PLACEHOLDER_RE = /(<uuid>|<path>|<obj>|<n>)/g;
const LEVEL_STYLES = {
    DEBUG: levelDebug,
    INFO: levelInfo,
    WARN: levelWarning,
    ERROR: levelError
};
/** Renders a normalized pattern string, styling `<placeholder>` tokens distinctly from literal text. */ function PatternText({ pattern }) {
    const parts = pattern.split(PLACEHOLDER_RE);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(react_jsx_runtime.Fragment, {
        children: parts.map((part, i)=>PLACEHOLDER_RE.test(part) ? /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                className: `${placeholder}`,
                children: part
            }, i) : /*#__PURE__*/ (0,react_jsx_runtime.jsx)(react.Fragment, {
                children: part
            }, i))
    });
}
/**
 * PatternsView — Datadog-style "Log Patterns" clustering for the app logs
 * page: groups the currently-loaded entries by normalized message (see
 * lib/logs/logPatterns.ts), most-frequent first. Expanding a row shows the
 * raw matching entries — including whatever the pattern's placeholders
 * normalized away (the actual path, UUID, or count).
 */ function PatternsView({ entries, maxExamplesPerPattern = 20 }) {
    const { track } = (0,analytics/* useAnalytics */.st)();
    const [expandedKey, setExpandedKey] = (0,react.useState)(null);
    const groups = (0,react.useMemo)(()=>groupLogsByPattern(entries), [
        entries
    ]);
    if (groups.length === 0) {
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
            className: `${PatternsView_css_container}`,
            children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: `${empty}`,
                children: "No log entries loaded to cluster into patterns."
            })
        });
    }
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        className: `${PatternsView_css_container}`,
        "data-testid": "patterns-view",
        children: groups.map((group)=>{
            const isOpen = expandedKey === group.key;
            return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: `${row}`,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: `${rowHeader}`,
                        role: "button",
                        tabIndex: 0,
                        "aria-expanded": isOpen,
                        onClick: ()=>{
                            track({
                                name: "log_pattern_toggled",
                                category: "user_action",
                                labels: {
                                    expanded: String(!isOpen)
                                }
                            });
                            setExpandedKey(isOpen ? null : group.key);
                        },
                        onKeyDown: (e)=>{
                            if (e.key === "Enter" || e.key === " ") {
                                e.preventDefault();
                                track({
                                    name: "log_pattern_toggled",
                                    category: "user_action",
                                    labels: {
                                        expanded: String(!isOpen)
                                    }
                                });
                                setExpandedKey(isOpen ? null : group.key);
                            }
                        },
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: `${disclosure} ${isOpen ? disclosureOpen : ""}`,
                                children: "▶"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: `${count}`,
                                children: group.count.toLocaleString()
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: `${level} ${LEVEL_STYLES[group.level] ?? ""}`,
                                children: group.level
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: `${pattern}`,
                                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(PatternText, {
                                    pattern: group.pattern
                                })
                            })
                        ]
                    }),
                    isOpen && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: `${examples}`,
                        children: [
                            group.entries.slice(0, maxExamplesPerPattern).map((entry)=>/*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                    className: `${example}`,
                                    children: [
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                            className: `${exampleTimestamp}`,
                                            children: (0,datetime/* formatTimestampShort */.CC)(new Date(entry.timestamp))
                                        }),
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                            className: `${exampleMessage}`,
                                            children: entry.message
                                        })
                                    ]
                                }, entry.id)),
                            group.entries.length > maxExamplesPerPattern && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                className: `${examplesMore}`,
                                children: [
                                    "…and ",
                                    (group.entries.length - maxExamplesPerPattern).toLocaleString(),
                                    " more"
                                ]
                            })
                        ]
                    })
                ]
            }, group.key);
        })
    });
}

// EXTERNAL MODULE: ./src/components/shared/LogViewer.tsx + 18 modules
var LogViewer = __webpack_require__(40201);
// EXTERNAL MODULE: ./src/components/ui/ActionBar.tsx + 1 modules
var ActionBar = __webpack_require__(89714);
// EXTERNAL MODULE: ./src/lib/analytics/usePageView.ts
var usePageView = __webpack_require__(15116);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fapp%2Flogs%2Fpage.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA71WXY_iIBR991eQeVqTwZQ66ojZX7LZbGi5tey00AD1azP_fVOqDu22VWdcX2zkwr2cwzkXJsU-mJo8QH9GCHFhioztKUoy2K1GyH0xFxpiK5SkKFZZmcsqkoJYp5aiDdPfMN4I2BZKW1wPPyMSBHyTjquZagM6ydSWolRwDm51wTgXck0Rmcw05NVQxOK3tVal5DhWmdKn1L_IEg6hPsxdss5Q4EKJkhYnLBfZvj0hLMer0ftoUmMl3Vh_l8aKZI9jJS1IS5EpWAw4AruFetcsE2uJhYXcUBSDtKCr4ZzptZA4Utaq_APSuV7YXa8n25oVFJFmhqnLcCYtcBWOky4zt3D0REpz0BSRYoeMygRvzQJvFtaMi9K0E03nH0QbcYB_4lH_GYV1qNSmiqWQFR6-l4fhG9DPaW-FEqez-BobsYtbzaQRtX3aSFAwCY3PA00rtzg2LqF-9SU965aYp6Vemfpm_AThfeTcciiuy2x1tdnq18M1v906Qdt-iy5xXQOTsKswvPULi0R3MFXcp86rG96CJioujeNBlTYTEiiSSoK3uc79Z36W1zaPL5VJg8myl04n_YJpkPaLDg3_p0Mv8XjRw2RmnpH3xyftRkcPdKhzzuVV2fLxpdPN--vNG_VuwCD9haypmPAoFAs7i52FffM-rumQYMCxDaYjB6DqOhSRvrdMLiQ-vYaC1W2vqOsePffqtGdcscN1vBCs8i4Jz96LxiV8v-fSw2-XXvuz4YZzposP3K5Hngbwfqo-8etDs_6Rcb_NwA_OLMO1KL8_WV3C08_mqrrfv_8FZzzPQ_ALAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fapp_2Flogs_2Fpage_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA71WXY_iIBR991eQeVqTwZQ66ojZX7LZbGi5tey00AD1azP_fVOqDu22VWdcX2zkwr2cwzkXJsU_mJo8QH9GCHFhioztKUoy2K1GyH0xFxpiK5SkKFZZmcsqkoJYp5aiDdPfMN4I2BZKW1wPPyMSBHyTjquZagM6ydSWolRwDm51wTgXck0Rmcw05NVQxOK3tVal5DhWmdKn1L_IEg6hPsxdss5Q4EKJkhYnLBfZvj0hLMer0ftoUmMl3Vh_l8aKZI9jJS1IS5EpWAw4AruFetcsE2uJhYXcUBSDtKCr4ZzptZA4Utaq_APSuV7YXa8n25oVFJFmhqnLcCYtcBWOky4zt3D0REpz0BSRYoeMygRvzQJvFtaMi9K0E03nH0QbcYB_4lH_GYV1qNSmiqWQFR6_l4fhG9DPaW_FEqez_BobsYtbzaQRtX3aSFAwCY3PA00rtzg2LqF_9SU965aYp6Vemfpm_AThfeTcciiuy2x1tdnq18M1v906Qdt_iy5xXQOTsKswvPULi0R3MFXcp86rG96CJioujeNBlTYTEiiSSoK3uc79Z36W1zaPL5VJg8myl04n_YJpkPaLDg3_p0Mv8XjRw2RmnpH3xyftRkcPdKhzzuVV2fLxpdPN_vNG_VuwCD9haypmPAoFAs7i52FffM_rumQYMCxDaYjB6DqOhSRvrdMLiQ_vYaC1W2vqOsePffqtGdcscN1vBCs8i4Jz96LxiV8v_fSw2_XXvuz4YZzposP3K5Hngbwfqo_8etDs_6Rcb_NwA_OLMO1KL8_WV3C08_mqrrfv_8FZzzPQ_ALAAA_22_7D = __webpack_require__(81559);
;// ./src/app/logs/page.css.ts


var page_css_container = 'py03sm0';
var error = 'py03sma';
var filterGroup = 'py03sm6';
var filters = 'py03sm5';
var footer = 'py03smc';
var header = 'py03sm1';
var headerActions = 'py03sm2';
var logsContainer = 'py03smb';
var refreshButton = 'py03sm4';
var page_css_select = 'py03sm7';
var shortcuts = 'py03smd';
var timezone = 'py03sm3';
var viewPane = 'py03sme';
var viewTab = 'py03sm8';
var viewTabActive = 'py03sm9 py03sm8';
;// ./src/app/logs/page.tsx
/* __next_internal_client_entry_do_not_use__ default auto */ 











function LogsPage() {
    (0,usePageView/* usePageView */.O)();
    const { track } = (0,analytics/* useAnalytics */.st)();
    // Time range and limit are the two real, wired filters for this page —
    // LogViewer owns search/level filtering itself (see LogViewerToolbar,
    // rendered inside LogViewer) since those need to apply to the same fetch
    // it's already doing; duplicating them here would just be a second,
    // easy-to-desync copy of the same state.
    const [timeRange, setTimeRange] = (0,react.useState)(()=>{
        const preset = datetime/* TIME_RANGE_PRESETS */.jX.find((p)=>p.value === "1h");
        const range = preset?.getRange() || {
            start: new Date(Date.now() - 60 * 60 * 1000),
            end: new Date()
        };
        return {
            ...range,
            preset: "1h"
        };
    });
    const [limit, setLimit] = (0,react.useState)(200);
    const [viewMode, setViewMode] = (0,react.useState)("table");
    // Lifted from LogViewer via onStateChange — this is the single source of
    // truth for what's actually displayed, used by Export, the Patterns view,
    // and the footer count.
    const [logs, setLogs] = (0,react.useState)([]);
    const [rawEntries, setRawEntries] = (0,react.useState)([]);
    const [serverTotalCount, setServerTotalCount] = (0,react.useState)(0);
    const [lastRefresh, setLastRefresh] = (0,react.useState)(null);
    const [logsError, setLogsError] = (0,react.useState)(null);
    const logViewerRef = (0,react.useRef)(null);
    const handleLogViewerStateChange = (0,react.useCallback)((state)=>{
        setLogs(state.logs);
        setRawEntries(state.rawEntries);
        setServerTotalCount(state.totalCount);
        setLastRefresh(state.lastRefresh);
        setLogsError(state.error);
    }, []);
    // "all" means no bound — everything else is a real time-bounded window.
    const activeTimeRange = timeRange.preset === "all" ? undefined : {
        start: timeRange.start,
        end: timeRange.end
    };
    const removeTimeRangeFilter = ()=>{
        const preset = datetime/* TIME_RANGE_PRESETS */.jX.find((p)=>p.value === "all");
        if (preset) {
            const range = preset.getRange();
            setTimeRange({
                ...range,
                preset: "all"
            });
        }
    };
    const hasActiveFilters = Boolean(timeRange.preset && timeRange.preset !== "all");
    // LogViewer already implements /, Escape, g, G, =, ?, Cmd+F, and j/k
    // internally (see its own handleKeyDown) for everything scoped to the log
    // list itself — only page-level actions belong here.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const isInputFocused = ()=>{
            const active = document.activeElement;
            return active instanceof HTMLInputElement || active instanceof HTMLTextAreaElement || active instanceof HTMLSelectElement;
        };
        const handleKeyDown = (e)=>{
            if (e.key === "r" && !isInputFocused()) {
                e.preventDefault();
                logViewerRef.current?.refresh();
            }
        };
        document.addEventListener("keydown", handleKeyDown);
        return ()=>document.removeEventListener("keydown", handleKeyDown);
    }, []);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("main", {
        id: "main-content",
        className: page_css_container,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("header", {
                className: header,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("h1", {
                        children: "Application Logs"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(ActionBar/* ActionBar */.E, {
                        scroll: true,
                        compact: true,
                        gap: "md",
                        className: headerActions,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(TimeRangePicker, {
                                value: timeRange,
                                onChange: setTimeRange
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: timezone,
                                title: "Your local timezone",
                                children: (0,datetime/* getUserTimezone */.dB)()
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                onClick: ()=>{
                                    track({
                                        name: "logs_refresh_clicked",
                                        category: "user_action"
                                    });
                                    logViewerRef.current?.refresh();
                                },
                                className: refreshButton,
                                "aria-label": "Refresh logs",
                                title: lastRefresh ? `Last updated: ${(0,datetime/* formatRelativeTime */.fw)(lastRefresh.getTime())}` : "Not yet refreshed",
                                children: "\uD83D\uDD04 Refresh"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ExportButton, {
                                logs: rawEntries,
                                disabled: rawEntries.length === 0
                            })
                        ]
                    })
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(ActionBar/* ActionBar */.E, {
                scroll: true,
                compact: true,
                gap: "md",
                className: filters,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: filterGroup,
                        role: "tablist",
                        "aria-label": "Log view",
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                role: "tab",
                                "aria-selected": viewMode === "table",
                                className: viewMode === "table" ? viewTabActive : viewTab,
                                onClick: ()=>{
                                    track({
                                        name: "logs_view_mode_changed",
                                        category: "user_action",
                                        labels: {
                                            mode: "table"
                                        }
                                    });
                                    setViewMode("table");
                                },
                                children: "Table"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                role: "tab",
                                "aria-selected": viewMode === "patterns",
                                className: viewMode === "patterns" ? viewTabActive : viewTab,
                                onClick: ()=>{
                                    track({
                                        name: "logs_view_mode_changed",
                                        category: "user_action",
                                        labels: {
                                            mode: "patterns"
                                        }
                                    });
                                    setViewMode("patterns");
                                },
                                children: "Patterns"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: filterGroup,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("label", {
                                htmlFor: "limit",
                                children: "Limit:"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("select", {
                                id: "limit",
                                value: limit,
                                onChange: (e)=>setLimit(Number(e.target.value)),
                                className: page_css_select,
                                "aria-label": "Results per page",
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                        value: "50",
                                        children: "50"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                        value: "100",
                                        children: "100"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                        value: "200",
                                        children: "200"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                        value: "500",
                                        children: "500"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                        value: "1000",
                                        children: "1000"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("option", {
                                        value: "2000",
                                        children: "2000"
                                    })
                                ]
                            })
                        ]
                    })
                ]
            }),
            hasActiveFilters && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(FilterPills, {
                onClearAll: removeTimeRangeFilter,
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(FilterPill, {
                    label: "Time",
                    value: datetime/* TIME_RANGE_PRESETS */.jX.find((p)=>p.value === timeRange.preset)?.label || "Custom",
                    onRemove: removeTimeRangeFilter
                })
            }),
            logsError && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: error,
                role: "alert",
                "data-testid": "logs-error-banner",
                children: [
                    "Error loading logs: ",
                    logsError
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: viewPane,
                "data-hidden": viewMode !== "table",
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                    className: logsContainer,
                    children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(LogViewer/* LogViewer */.r, {
                        ref: logViewerRef,
                        source: "app",
                        timeRange: activeTimeRange,
                        limit: limit,
                        onStateChange: handleLogViewerStateChange
                    })
                })
            }),
            viewMode === "patterns" && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: logsContainer,
                children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)(PatternsView, {
                    entries: logs
                })
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("footer", {
                className: footer,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                        children: [
                            "Showing ",
                            logs.length,
                            " of ",
                            serverTotalCount,
                            " log entries"
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                        className: shortcuts,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("kbd", {
                                children: "R"
                            }),
                            " Refresh • ",
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("kbd", {
                                children: "/"
                            }),
                            " Search • ",
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("kbd", {
                                children: "?"
                            }),
                            " All shortcuts"
                        ]
                    })
                ]
            })
        ]
    });
}


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

/***/ 76525:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
__webpack_require__.r(__webpack_exports__);
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   "default": () => (__WEBPACK_DEFAULT_EXPORT__)
/* harmony export */ });
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(70679);
/* harmony import */ var react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__);

/* harmony default export */ const __WEBPACK_DEFAULT_EXPORT__ = ((0,react_server_dom_webpack_server_edge__WEBPACK_IMPORTED_MODULE_0__.registerClientReference)(
function() { throw new Error("Attempted to call the default export of \"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/logs/page.tsx\" from the server, but it's on the client. It's not possible to invoke a client function from the server, it can only be rendered as a Component or passed to props of a Client Component."); },
"/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/triage-46b6f173-907a-4f07-91d8-3fd1f3831430_18e0e70f28df34e4/web-app/src/app/logs/page.tsx",
"default",
));


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

/***/ 81559:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".py03sm0 {\n  display: flex;\n  flex-direction: column;\n  height: var(--viewport-height, 100dvh);\n  overflow: hidden;\n  padding: 1.5rem;\n  background-color: var(--_19ez2rz6);\n  color: var(--_19ez2rz0);\n  font-family: var(--_19ez2rz2u);\n}\n.py03sm1 {\n  display: flex;\n  justify-content: space-between;\n  align-items: center;\n  margin-bottom: 1.5rem;\n}\n.py03sm2 {\n  display: flex;\n  align-items: center;\n  gap: 1rem;\n}\n.py03sm3 {\n  padding: 0.5rem 1rem;\n  background-color: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz2);\n  cursor: help;\n}\n.py03sm4 {\n  padding: 0.5rem 1rem;\n  background-color: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  color: var(--_19ez2rz0);\n  cursor: pointer;\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3c);\n  transition: background-color 0.2s;\n}\n.py03sm4:hover {\n  background-color: var(--_19ez2rz8);\n}\n.py03sm5 {\n  display: flex;\n  gap: 1rem;\n  margin-bottom: 1.5rem;\n  padding: 1rem;\n  background-color: var(--_19ez2rz7);\n  border-radius: var(--_19ez2rz37);\n  border: 1px solid var(--_19ez2rze);\n  flex-wrap: wrap;\n}\n.py03sm6 {\n  display: flex;\n  align-items: center;\n  gap: 0.5rem;\n}\n.py03sm7 {\n  padding: 0.5rem;\n  background-color: var(--_19ez2rz1a);\n  border: 1px solid var(--_19ez2rzk);\n  color: var(--_19ez2rz1b);\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3c);\n  cursor: pointer;\n  font-family: var(--_19ez2rz2u);\n}\n.py03sm7:focus {\n  outline: none;\n  border-color: var(--_19ez2rzl);\n}\n.py03sm8 {\n  padding: 0.4rem 0.9rem;\n  background-color: transparent;\n  border: 1px solid var(--_19ez2rze);\n  color: var(--_19ez2rz2);\n  cursor: pointer;\n  border-radius: var(--_19ez2rz36);\n  font-size: var(--_19ez2rz3c);\n  font-family: var(--_19ez2rz2u);\n  transition: background-color 0.15s, color 0.15s;\n}\n.py03sm8:hover {\n  background-color: var(--_19ez2rz8);\n  color: var(--_19ez2rz0);\n}\n.py03sm9 {\n  background-color: var(--_19ez2rzm);\n  border-color: var(--_19ez2rzm);\n  color: var(--_19ez2rz6);\n}\n.py03sm9:hover {\n  background-color: var(--_19ez2rzn);\n}\n.py03sma {\n  padding: 2rem;\n  text-align: center;\n  background-color: var(--_19ez2rz7);\n  border-radius: var(--_19ez2rz37);\n  border: 1px solid var(--_19ez2rz10);\n  color: var(--_19ez2rz10);\n}\n.py03smb {\n  flex: 1 1;\n  overflow: hidden;\n  min-height: 0;\n  display: flex;\n  flex-direction: column;\n  background-color: var(--_19ez2rz6);\n  border-radius: var(--_19ez2rz37);\n  border: 1px solid var(--_19ez2rze);\n}\n.py03smc {\n  margin-top: 1rem;\n  padding: 0.75rem 1rem;\n  display: flex;\n  justify-content: space-between;\n  align-items: center;\n  background-color: var(--_19ez2rz7);\n  border-radius: var(--_19ez2rz37);\n  border: 1px solid var(--_19ez2rze);\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz2);\n}\n.py03smd {\n  display: flex;\n  gap: 0.75rem;\n  align-items: center;\n  font-size: var(--_19ez2rz3a);\n  color: var(--_19ez2rz1);\n}\n.py03sme {\n  display: contents;\n}\n.py03sme[data-hidden=\"true\"] {\n  display: none;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


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

/***/ 85021:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._142mmks0 {\n  position: relative;\n}\n._142mmks1 {\n  display: flex;\n  align-items: center;\n  gap: 0.5rem;\n  padding: 0.5rem 0.75rem;\n  background-color: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rzk);\n  color: var(--_19ez2rz0);\n  border-radius: 4px;\n  cursor: pointer;\n  font-size: 0.85rem;\n  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;\n  transition: all 0.2s;\n}\n._142mmks1:hover:not(:disabled) {\n  background-color: var(--_19ez2rz8);\n  border-color: var(--_19ez2rz2);\n}\n._142mmks1:disabled {\n  opacity: 0.5;\n  cursor: not-allowed;\n}\n._142mmks2 {\n  position: absolute;\n  top: 100%;\n  right: 0;\n  margin-top: 4px;\n  background-color: var(--_19ez2rz7);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: 6px;\n  box-shadow: var(--_19ez2rz3l);\n  z-index: 100;\n  min-width: 180px;\n  overflow: hidden;\n}\n._142mmks3 {\n  display: flex;\n  align-items: center;\n  gap: 0.5rem;\n  padding: 0.625rem 0.75rem;\n  background: none;\n  border: none;\n  color: var(--_19ez2rz0);\n  cursor: pointer;\n  font-size: 0.85rem;\n  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;\n  width: 100%;\n  text-align: left;\n  transition: background-color 0.15s;\n}\n._142mmks3:hover {\n  background-color: var(--_19ez2rz8);\n}\n._142mmks4 {\n  font-size: 0.9rem;\n  width: 20px;\n  text-align: center;\n}\n._142mmks5 {\n  font-weight: 500;\n}\n._142mmks6 {\n  font-size: 0.75rem;\n  color: var(--_19ez2rz2);\n  margin-left: auto;\n}\n._142mmks7 {\n  height: 1px;\n  background-color: var(--_19ez2rze);\n  margin: 0.25rem 0;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 86642:
/***/ ((__unused_webpack_module, __unused_webpack_exports, __webpack_require__) => {

Promise.resolve(/* import() eager */).then(__webpack_require__.bind(__webpack_require__, 67397));


/***/ }),

/***/ 89714:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  E: () => (/* binding */ ActionBar)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fui%2FActionBar.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8WSy26DMBBF93zFLJOqRiTN05Gq_kk1wIS4gTGynUBa5d8rIKDQvKpuusFifOb6-nr891GZh8ifAXx5ALGyeYoHCeuUypUH9SoKg7mE6luVMFUJC-UosxIiYkemKicVs0czEAIjpzSLEI1IMH-GwJ8ayoYr7-j57Xmj-ryfrITAH1dwjx3fZC_Ql5vo_IKd1OzHzjq1PohIsyN2zdWFdWhcj57eoYnjHju7ztocIxIhuYKIew3z6w1tukfvLaNYIdjIEDEgxzDIsBSFit1GwigYT_JyWIt0mov69_QwXawAx4dy89niptrZQLBuRwJA78msU12IUgLunG6qoqBwq5zodm1kdJoqTiQ4vYs2DdZUq7c6GWDN1Fg9dyBlq9c1nEx1Y3u9EV7h6dy-3RjFWwnB7-KYTYKLOJb_Hsfyr3EsH8XxDcl0doYUBAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fui_2FActionBar_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8WSy26DMBBF93zFLJOqRiTN05Gq_kk1wIS4gTGynUBa5d8rIKDQvKpuusFifOb6_nr891GZh8ifAXx5ALGyeYoHCeuUypUH9SoKg7mE6luVMFUJC_UosxIiYkemKicVs0czEAIjpzSLEI1IMH_GwJ8ayoYr7_j57Xmj_ryfrITAH1dwjx3fZC_Ql5vo_IKd1OzHzjq1PohIsyN2zdWFdWhcj57eoYnjHju7ztocIxIhuYKIew3z6w1tukfvLaNYIdjIEDEgxzDIsBSFit1GwigYT_JyWIt0mov69_QwXawAx4dy89niptrZQLBuRwJA78msU12IUgLunG6qoqBwq5zodm1kdJoqTiQ4vYs2DdZUq7c6GWDN1Fg9dyBlq9c1nEx1Y3u9EV7h6dy_3RjFWwnB7_KYTYKLOJb_Hsfyr3EsH8XxDcl0doYUBAAA_22_7D = __webpack_require__(10197);
;// ./src/components/ui/ActionBar.css.ts

var actionBar = '_1xpbanz0';
var ActionBar_css_compact = '_1xpbanz8';
var gapLg = '_1xpbanz3';
var gapMd = '_1xpbanz2';
var gapSm = '_1xpbanz1';
var justifyBetween = '_1xpbanz6';
var justifyCenter = '_1xpbanz7';
var justifyEnd = '_1xpbanz5';
var justifyStart = '_1xpbanz4';
var ActionBar_css_scroll = '_1xpbanz9';
;// ./src/components/ui/ActionBar.tsx
/* __next_internal_client_entry_do_not_use__ ActionBar auto */ 

const gapClass = {
    sm: gapSm,
    md: gapMd,
    lg: gapLg
};
const justifyClass = {
    start: justifyStart,
    end: justifyEnd,
    between: justifyBetween,
    center: justifyCenter
};
function ActionBar({ children, gap = "md", justify = "start", scroll, compact, className, id }) {
    const classes = [
        actionBar,
        gapClass[gap],
        justifyClass[justify],
        scroll ? ActionBar_css_scroll : undefined,
        compact ? ActionBar_css_compact : undefined,
        className
    ].filter(Boolean).join(" ");
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        id: id,
        className: classes,
        children: children
    });
}


/***/ }),

/***/ 94266:
/***/ (() => {



/***/ })

};
;

// load runtime
var __webpack_require__ = require("../../webpack-runtime.js");
__webpack_require__.C(exports);
var __webpack_exec__ = (moduleId) => (__webpack_require__(__webpack_require__.s = moduleId))
var __webpack_exports__ = __webpack_require__.X(0, [4655,3258,1911,8809,4164,201], () => (__webpack_exec__(13370)));
module.exports = __webpack_exports__;

})();