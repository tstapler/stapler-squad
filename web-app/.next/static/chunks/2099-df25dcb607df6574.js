"use strict";
(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[2099],{

/***/ 52029:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  _: () => (/* binding */ ContextAPI)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/context/context.js
var context = __webpack_require__(82902);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/context/NoopContextManager.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

class NoopContextManager {
    active() {
        return context/* ROOT_CONTEXT */.l;
    }
    with(_context, fn, thisArg, ...args) {
        return fn.call(thisArg, ...args);
    }
    bind(_context, target) {
        return target;
    }
    enable() {
        return this;
    }
    disable() {
        return this;
    }
}
//# sourceMappingURL=NoopContextManager.js.map
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/internal/global-utils.js + 2 modules
var global_utils = __webpack_require__(59556);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/api/diag.js + 3 modules
var diag = __webpack_require__(53151);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/api/context.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



const API_NAME = 'context';
const NOOP_CONTEXT_MANAGER = new NoopContextManager();
/**
 * Singleton object which represents the entry point to the OpenTelemetry Context API
 *
 * @since 1.0.0
 */
class ContextAPI {
    /** Empty private constructor prevents end users from constructing a new instance of the API */
    constructor() { }
    /** Get the singleton instance of the Context API */
    static getInstance() {
        if (!this._instance) {
            this._instance = new ContextAPI();
        }
        return this._instance;
    }
    /**
     * Set the current context manager.
     *
     * @returns true if the context manager was successfully registered, else false
     */
    setGlobalContextManager(contextManager) {
        return (0,global_utils/* registerGlobal */.$G)(API_NAME, contextManager, diag/* DiagAPI */.K.instance());
    }
    /**
     * Get the currently active context
     */
    active() {
        return this._getContextManager().active();
    }
    /**
     * Execute a function with an active context
     *
     * @param context context to be active during function execution
     * @param fn function to execute in a context
     * @param thisArg optional receiver to be used for calling fn
     * @param args optional arguments forwarded to fn
     */
    with(context, fn, thisArg, ...args) {
        return this._getContextManager().with(context, fn, thisArg, ...args);
    }
    /**
     * Bind a context to a target function or event emitter
     *
     * @param context context to bind to the event emitter or function. Defaults to the currently active context
     * @param target function or event emitter to bind
     */
    bind(context, target) {
        return this._getContextManager().bind(context, target);
    }
    _getContextManager() {
        return (0,global_utils/* getGlobal */.mS)(API_NAME) || NOOP_CONTEXT_MANAGER;
    }
    /** Disable and remove the global context manager */
    disable() {
        this._getContextManager().disable();
        (0,global_utils/* unregisterGlobal */.kv)(API_NAME, diag/* DiagAPI */.K.instance());
    }
}
//# sourceMappingURL=context.js.map

/***/ }),

/***/ 53151:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  K: () => (/* binding */ DiagAPI)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/internal/global-utils.js + 2 modules
var global_utils = __webpack_require__(59556);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/diag/ComponentLogger.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * Component Logger which is meant to be used as part of any component which
 * will add automatically additional namespace in front of the log message.
 * It will then forward all message to global diag logger
 * @example
 * const cLogger = diag.createComponentLogger({ namespace: '@opentelemetry/instrumentation-http' });
 * cLogger.debug('test');
 * // @opentelemetry/instrumentation-http test
 */
class DiagComponentLogger {
    constructor(props) {
        this._namespace = props.namespace || 'DiagComponentLogger';
    }
    debug(...args) {
        return logProxy('debug', this._namespace, args);
    }
    error(...args) {
        return logProxy('error', this._namespace, args);
    }
    info(...args) {
        return logProxy('info', this._namespace, args);
    }
    warn(...args) {
        return logProxy('warn', this._namespace, args);
    }
    verbose(...args) {
        return logProxy('verbose', this._namespace, args);
    }
}
function logProxy(funcName, namespace, args) {
    const logger = (0,global_utils/* getGlobal */.mS)('diag');
    // shortcut if logger not set
    if (!logger) {
        return;
    }
    return logger[funcName](namespace, ...args);
}
//# sourceMappingURL=ComponentLogger.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/diag/types.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * Defines the available internal logging levels for the diagnostic logger, the numeric values
 * of the levels are defined to match the original values from the initial LogLevel to avoid
 * compatibility/migration issues for any implementation that assume the numeric ordering.
 */
var DiagLogLevel;
(function (DiagLogLevel) {
    /** Diagnostic Logging level setting to disable all logging (except and forced logs) */
    DiagLogLevel[DiagLogLevel["NONE"] = 0] = "NONE";
    /** Identifies an error scenario */
    DiagLogLevel[DiagLogLevel["ERROR"] = 30] = "ERROR";
    /** Identifies a warning scenario */
    DiagLogLevel[DiagLogLevel["WARN"] = 50] = "WARN";
    /** General informational log message */
    DiagLogLevel[DiagLogLevel["INFO"] = 60] = "INFO";
    /** General debug log message */
    DiagLogLevel[DiagLogLevel["DEBUG"] = 70] = "DEBUG";
    /**
     * Detailed trace level logging should only be used for development, should only be set
     * in a development environment.
     */
    DiagLogLevel[DiagLogLevel["VERBOSE"] = 80] = "VERBOSE";
    /** Used to set the logging level to include all logging */
    DiagLogLevel[DiagLogLevel["ALL"] = 9999] = "ALL";
})(DiagLogLevel || (DiagLogLevel = {}));
//# sourceMappingURL=types.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/diag/internal/logLevelLogger.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

function createLogLevelDiagLogger(maxLevel, logger) {
    if (maxLevel < DiagLogLevel.NONE) {
        maxLevel = DiagLogLevel.NONE;
    }
    else if (maxLevel > DiagLogLevel.ALL) {
        maxLevel = DiagLogLevel.ALL;
    }
    // In case the logger is null or undefined
    logger = logger || {};
    function _filterFunc(funcName, theLevel) {
        const theFunc = logger[funcName];
        if (typeof theFunc === 'function' && maxLevel >= theLevel) {
            return theFunc.bind(logger);
        }
        return function () { };
    }
    return {
        error: _filterFunc('error', DiagLogLevel.ERROR),
        warn: _filterFunc('warn', DiagLogLevel.WARN),
        info: _filterFunc('info', DiagLogLevel.INFO),
        debug: _filterFunc('debug', DiagLogLevel.DEBUG),
        verbose: _filterFunc('verbose', DiagLogLevel.VERBOSE),
    };
}
//# sourceMappingURL=logLevelLogger.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/api/diag.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */




const API_NAME = 'diag';
/**
 * Singleton object which represents the entry point to the OpenTelemetry internal
 * diagnostic API
 *
 * @since 1.0.0
 */
class DiagAPI {
    /** Get the singleton instance of the DiagAPI API */
    static instance() {
        if (!this._instance) {
            this._instance = new DiagAPI();
        }
        return this._instance;
    }
    /**
     * Private internal constructor
     * @private
     */
    constructor() {
        function _logProxy(funcName) {
            return function (...args) {
                const logger = (0,global_utils/* getGlobal */.mS)('diag');
                // shortcut if logger not set
                if (!logger)
                    return;
                return logger[funcName](...args);
            };
        }
        // Using self local variable for minification purposes as 'this' cannot be minified
        const self = this;
        // DiagAPI specific functions
        const setLogger = (logger, optionsOrLogLevel = { logLevel: DiagLogLevel.INFO }) => {
            var _a, _b, _c;
            if (logger === self) {
                // There isn't much we can do here.
                // Logging to the console might break the user application.
                // Try to log to self. If a logger was previously registered it will receive the log.
                const err = new Error('Cannot use diag as the logger for itself. Please use a DiagLogger implementation like ConsoleDiagLogger or a custom implementation');
                self.error((_a = err.stack) !== null && _a !== void 0 ? _a : err.message);
                return false;
            }
            if (typeof optionsOrLogLevel === 'number') {
                optionsOrLogLevel = {
                    logLevel: optionsOrLogLevel,
                };
            }
            const oldLogger = (0,global_utils/* getGlobal */.mS)('diag');
            const newLogger = createLogLevelDiagLogger((_b = optionsOrLogLevel.logLevel) !== null && _b !== void 0 ? _b : DiagLogLevel.INFO, logger);
            // There already is an logger registered. We'll let it know before overwriting it.
            if (oldLogger && !optionsOrLogLevel.suppressOverrideMessage) {
                const stack = (_c = new Error().stack) !== null && _c !== void 0 ? _c : '<failed to generate stacktrace>';
                oldLogger.warn(`Current logger will be overwritten from ${stack}`);
                newLogger.warn(`Current logger will overwrite one already registered from ${stack}`);
            }
            return (0,global_utils/* registerGlobal */.$G)('diag', newLogger, self, true);
        };
        self.setLogger = setLogger;
        self.disable = () => {
            (0,global_utils/* unregisterGlobal */.kv)(API_NAME, self);
        };
        self.createComponentLogger = (options) => {
            return new DiagComponentLogger(options);
        };
        self.verbose = _logProxy('verbose');
        self.debug = _logProxy('debug');
        self.info = _logProxy('info');
        self.warn = _logProxy('warn');
        self.error = _logProxy('error');
    }
}
//# sourceMappingURL=diag.js.map

/***/ }),

/***/ 59556:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  mS: () => (/* binding */ getGlobal),
  $G: () => (/* binding */ registerGlobal),
  kv: () => (/* binding */ unregisterGlobal)
});

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/version.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// this is autogenerated file, see scripts/version-update.js
const VERSION = '1.9.1';
//# sourceMappingURL=version.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/internal/semver.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

const re = /^(\d+)\.(\d+)\.(\d+)(-(.+))?$/;
/**
 * Create a function to test an API version to see if it is compatible with the provided ownVersion.
 *
 * The returned function has the following semantics:
 * - Exact match is always compatible
 * - Major versions must match exactly
 *    - 1.x package cannot use global 2.x package
 *    - 2.x package cannot use global 1.x package
 * - The minor version of the API module requesting access to the global API must be less than or equal to the minor version of this API
 *    - 1.3 package may use 1.4 global because the later global contains all functions 1.3 expects
 *    - 1.4 package may NOT use 1.3 global because it may try to call functions which don't exist on 1.3
 * - If the major version is 0, the minor version is treated as the major and the patch is treated as the minor
 * - Patch and build tag differences are not considered at this time
 *
 * @param ownVersion version which should be checked against
 */
function _makeCompatibilityCheck(ownVersion) {
    const acceptedVersions = new Set([ownVersion]);
    const rejectedVersions = new Set();
    const myVersionMatch = ownVersion.match(re);
    if (!myVersionMatch) {
        // we cannot guarantee compatibility so we always return noop
        return () => false;
    }
    const ownVersionParsed = {
        major: +myVersionMatch[1],
        minor: +myVersionMatch[2],
        patch: +myVersionMatch[3],
        prerelease: myVersionMatch[4],
    };
    // if ownVersion has a prerelease tag, versions must match exactly
    if (ownVersionParsed.prerelease != null) {
        return function isExactmatch(globalVersion) {
            return globalVersion === ownVersion;
        };
    }
    function _reject(v) {
        rejectedVersions.add(v);
        return false;
    }
    function _accept(v) {
        acceptedVersions.add(v);
        return true;
    }
    return function isCompatible(globalVersion) {
        if (acceptedVersions.has(globalVersion)) {
            return true;
        }
        if (rejectedVersions.has(globalVersion)) {
            return false;
        }
        const globalVersionMatch = globalVersion.match(re);
        if (!globalVersionMatch) {
            // cannot parse other version
            // we cannot guarantee compatibility so we always noop
            return _reject(globalVersion);
        }
        const globalVersionParsed = {
            major: +globalVersionMatch[1],
            minor: +globalVersionMatch[2],
            patch: +globalVersionMatch[3],
            prerelease: globalVersionMatch[4],
        };
        // if globalVersion has a prerelease tag, versions must match exactly
        if (globalVersionParsed.prerelease != null) {
            return _reject(globalVersion);
        }
        // major versions must match
        if (ownVersionParsed.major !== globalVersionParsed.major) {
            return _reject(globalVersion);
        }
        if (ownVersionParsed.major === 0) {
            if (ownVersionParsed.minor === globalVersionParsed.minor &&
                ownVersionParsed.patch <= globalVersionParsed.patch) {
                return _accept(globalVersion);
            }
            return _reject(globalVersion);
        }
        if (ownVersionParsed.minor <= globalVersionParsed.minor) {
            return _accept(globalVersion);
        }
        return _reject(globalVersion);
    };
}
/**
 * Test an API version to see if it is compatible with this API.
 *
 * - Exact match is always compatible
 * - Major versions must match exactly
 *    - 1.x package cannot use global 2.x package
 *    - 2.x package cannot use global 1.x package
 * - The minor version of the API module requesting access to the global API must be less than or equal to the minor version of this API
 *    - 1.3 package may use 1.4 global because the later global contains all functions 1.3 expects
 *    - 1.4 package may NOT use 1.3 global because it may try to call functions which don't exist on 1.3
 * - If the major version is 0, the minor version is treated as the major and the patch is treated as the minor
 * - Patch and build tag differences are not considered at this time
 *
 * @param version version of the API requesting an instance of the global API
 */
const isCompatible = _makeCompatibilityCheck(VERSION);
//# sourceMappingURL=semver.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/internal/global-utils.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


const major = VERSION.split('.')[0];
const GLOBAL_OPENTELEMETRY_API_KEY = Symbol.for(`opentelemetry.js.api.${major}`);
const _global = (typeof globalThis === 'object'
    ? globalThis
    : typeof self === 'object'
        ? self
        : typeof window === 'object'
            ? window
            : typeof __webpack_require__.g === 'object'
                ? __webpack_require__.g
                : {});
function registerGlobal(type, instance, diag, allowOverride = false) {
    var _a;
    const api = (_global[GLOBAL_OPENTELEMETRY_API_KEY] = (_a = _global[GLOBAL_OPENTELEMETRY_API_KEY]) !== null && _a !== void 0 ? _a : {
        version: VERSION,
    });
    if (!allowOverride && api[type]) {
        // already registered an API of this type
        const err = new Error(`@opentelemetry/api: Attempted duplicate registration of API: ${type}`);
        diag.error(err.stack || err.message);
        return false;
    }
    if (api.version !== VERSION) {
        // All registered APIs must be of the same version exactly
        const err = new Error(`@opentelemetry/api: Registration of version v${api.version} for ${type} does not match previously registered API v${VERSION}`);
        diag.error(err.stack || err.message);
        return false;
    }
    api[type] = instance;
    diag.debug(`@opentelemetry/api: Registered a global for ${type} v${VERSION}.`);
    return true;
}
function getGlobal(type) {
    var _a, _b;
    const globalVersion = (_a = _global[GLOBAL_OPENTELEMETRY_API_KEY]) === null || _a === void 0 ? void 0 : _a.version;
    if (!globalVersion || !isCompatible(globalVersion)) {
        return;
    }
    return (_b = _global[GLOBAL_OPENTELEMETRY_API_KEY]) === null || _b === void 0 ? void 0 : _b[type];
}
function unregisterGlobal(type, diag) {
    diag.debug(`@opentelemetry/api: Unregistering a global for ${type} v${VERSION}.`);
    const api = _global[GLOBAL_OPENTELEMETRY_API_KEY];
    if (api) {
        delete api[type];
    }
}
//# sourceMappingURL=global-utils.js.map

/***/ }),

/***/ 66659:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   RH: () => (/* binding */ INVALID_TRACEID),
/* harmony export */   dM: () => (/* binding */ INVALID_SPAN_CONTEXT),
/* harmony export */   w9: () => (/* binding */ INVALID_SPANID)
/* harmony export */ });
/* harmony import */ var _trace_flags__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(76090);
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * @since 1.0.0
 */
const INVALID_SPANID = '0000000000000000';
/**
 * @since 1.0.0
 */
const INVALID_TRACEID = '00000000000000000000000000000000';
/**
 * @since 1.0.0
 */
const INVALID_SPAN_CONTEXT = {
    traceId: INVALID_TRACEID,
    spanId: INVALID_SPANID,
    traceFlags: _trace_flags__WEBPACK_IMPORTED_MODULE_0__/* .TraceFlags */ .X.NONE,
};
//# sourceMappingURL=invalid-span-constants.js.map

/***/ }),

/***/ 76090:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   X: () => (/* binding */ TraceFlags)
/* harmony export */ });
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * @since 1.0.0
 */
var TraceFlags;
(function (TraceFlags) {
    /** Represents no flag set. */
    TraceFlags[TraceFlags["NONE"] = 0] = "NONE";
    /** Bit to represent whether trace is sampled in trace flags. */
    TraceFlags[TraceFlags["SAMPLED"] = 1] = "SAMPLED";
})(TraceFlags || (TraceFlags = {}));
//# sourceMappingURL=trace_flags.js.map

/***/ }),

/***/ 82099:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {


// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  u: () => (/* binding */ trace)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/internal/global-utils.js + 2 modules
var global_utils = __webpack_require__(59556);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/api/context.js + 1 modules
var context = __webpack_require__(52029);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/context/context.js
var context_context = __webpack_require__(82902);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace/NonRecordingSpan.js
var NonRecordingSpan = __webpack_require__(88335);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace/context-utils.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */



/**
 * span key
 */
const SPAN_KEY = (0,context_context/* createContextKey */.n)('OpenTelemetry Context Key SPAN');
/**
 * Return the span if one exists
 *
 * @param context context to get span from
 */
function getSpan(context) {
    return context.getValue(SPAN_KEY) || undefined;
}
/**
 * Gets the span from the current context, if one exists.
 */
function getActiveSpan() {
    return getSpan(context/* ContextAPI */._.getInstance().active());
}
/**
 * Set the span on a context
 *
 * @param context context to use as parent
 * @param span span to set active
 */
function setSpan(context, span) {
    return context.setValue(SPAN_KEY, span);
}
/**
 * Remove current span stored in the context
 *
 * @param context context to delete span from
 */
function deleteSpan(context) {
    return context.deleteValue(SPAN_KEY);
}
/**
 * Wrap span context in a NoopSpan and set as span in a new
 * context
 *
 * @param context context to set active span on
 * @param spanContext span context to be wrapped
 */
function setSpanContext(context, spanContext) {
    return setSpan(context, new NonRecordingSpan/* NonRecordingSpan */.d(spanContext));
}
/**
 * Get the span context of the span if it exists.
 *
 * @param context context to get values from
 */
function getSpanContext(context) {
    var _a;
    return (_a = getSpan(context)) === null || _a === void 0 ? void 0 : _a.spanContext();
}
//# sourceMappingURL=context-utils.js.map
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace/spancontext-utils.js
var spancontext_utils = __webpack_require__(92268);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace/NoopTracer.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */




const contextApi = context/* ContextAPI */._.getInstance();
/**
 * No-op implementations of {@link Tracer}.
 */
class NoopTracer {
    // startSpan starts a noop span.
    startSpan(name, options, context = contextApi.active()) {
        const root = Boolean(options === null || options === void 0 ? void 0 : options.root);
        if (root) {
            return new NonRecordingSpan/* NonRecordingSpan */.d();
        }
        const parentFromContext = context && getSpanContext(context);
        if (isSpanContext(parentFromContext) &&
            (0,spancontext_utils/* isSpanContextValid */.YA)(parentFromContext)) {
            return new NonRecordingSpan/* NonRecordingSpan */.d(parentFromContext);
        }
        else {
            return new NonRecordingSpan/* NonRecordingSpan */.d();
        }
    }
    startActiveSpan(name, arg2, arg3, arg4) {
        let opts;
        let ctx;
        let fn;
        if (arguments.length < 2) {
            return;
        }
        else if (arguments.length === 2) {
            fn = arg2;
        }
        else if (arguments.length === 3) {
            opts = arg2;
            fn = arg3;
        }
        else {
            opts = arg2;
            ctx = arg3;
            fn = arg4;
        }
        const parentContext = ctx !== null && ctx !== void 0 ? ctx : contextApi.active();
        const span = this.startSpan(name, opts, parentContext);
        const contextWithSpanSet = setSpan(parentContext, span);
        return contextApi.with(contextWithSpanSet, fn, undefined, span);
    }
}
function isSpanContext(spanContext) {
    return (spanContext !== null &&
        typeof spanContext === 'object' &&
        'spanId' in spanContext &&
        typeof spanContext['spanId'] === 'string' &&
        'traceId' in spanContext &&
        typeof spanContext['traceId'] === 'string' &&
        'traceFlags' in spanContext &&
        typeof spanContext['traceFlags'] === 'number');
}
//# sourceMappingURL=NoopTracer.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace/ProxyTracer.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

const NOOP_TRACER = new NoopTracer();
/**
 * Proxy tracer provided by the proxy tracer provider
 *
 * @since 1.0.0
 */
class ProxyTracer {
    constructor(provider, name, version, options) {
        this._provider = provider;
        this.name = name;
        this.version = version;
        this.options = options;
    }
    startSpan(name, options, context) {
        return this._getTracer().startSpan(name, options, context);
    }
    startActiveSpan(_name, _options, _context, _fn) {
        const tracer = this._getTracer();
        return Reflect.apply(tracer.startActiveSpan, tracer, arguments);
    }
    /**
     * Try to get a tracer from the proxy tracer provider.
     * If the proxy tracer provider has no delegate, return a noop tracer.
     */
    _getTracer() {
        if (this._delegate) {
            return this._delegate;
        }
        const tracer = this._provider.getDelegateTracer(this.name, this.version, this.options);
        if (!tracer) {
            return NOOP_TRACER;
        }
        this._delegate = tracer;
        return this._delegate;
    }
}
//# sourceMappingURL=ProxyTracer.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace/NoopTracerProvider.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * An implementation of the {@link TracerProvider} which returns an impotent
 * Tracer for all calls to `getTracer`.
 *
 * All operations are no-op.
 */
class NoopTracerProvider {
    getTracer(_name, _version, _options) {
        return new NoopTracer();
    }
}
//# sourceMappingURL=NoopTracerProvider.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace/ProxyTracerProvider.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


const NOOP_TRACER_PROVIDER = new NoopTracerProvider();
/**
 * Tracer provider which provides {@link ProxyTracer}s.
 *
 * Before a delegate is set, tracers provided are NoOp.
 *   When a delegate is set, traces are provided from the delegate.
 *   When a delegate is set after tracers have already been provided,
 *   all tracers already provided will use the provided delegate implementation.
 *
 * @deprecated This will be removed in the next major version.
 * @since 1.0.0
 */
class ProxyTracerProvider {
    /**
     * Get a {@link ProxyTracer}
     */
    getTracer(name, version, options) {
        var _a;
        return ((_a = this.getDelegateTracer(name, version, options)) !== null && _a !== void 0 ? _a : new ProxyTracer(this, name, version, options));
    }
    getDelegate() {
        var _a;
        return (_a = this._delegate) !== null && _a !== void 0 ? _a : NOOP_TRACER_PROVIDER;
    }
    /**
     * Set the delegate tracer provider
     */
    setDelegate(delegate) {
        this._delegate = delegate;
    }
    getDelegateTracer(name, version, options) {
        var _a;
        return (_a = this._delegate) === null || _a === void 0 ? void 0 : _a.getTracer(name, version, options);
    }
}
//# sourceMappingURL=ProxyTracerProvider.js.map
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/api/diag.js + 3 modules
var diag = __webpack_require__(53151);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/api/trace.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */





const API_NAME = 'trace';
/**
 * Singleton object which represents the entry point to the OpenTelemetry Tracing API
 *
 * @since 1.0.0
 */
class TraceAPI {
    /** Empty private constructor prevents end users from constructing a new instance of the API */
    constructor() {
        this._proxyTracerProvider = new ProxyTracerProvider();
        this.wrapSpanContext = spancontext_utils/* wrapSpanContext */.IP;
        this.isSpanContextValid = spancontext_utils/* isSpanContextValid */.YA;
        this.deleteSpan = deleteSpan;
        this.getSpan = getSpan;
        this.getActiveSpan = getActiveSpan;
        this.getSpanContext = getSpanContext;
        this.setSpan = setSpan;
        this.setSpanContext = setSpanContext;
    }
    /** Get the singleton instance of the Trace API */
    static getInstance() {
        if (!this._instance) {
            this._instance = new TraceAPI();
        }
        return this._instance;
    }
    /**
     * Set the current global tracer.
     *
     * @returns true if the tracer provider was successfully registered, else false
     */
    setGlobalTracerProvider(provider) {
        const success = (0,global_utils/* registerGlobal */.$G)(API_NAME, this._proxyTracerProvider, diag/* DiagAPI */.K.instance());
        if (success) {
            this._proxyTracerProvider.setDelegate(provider);
        }
        return success;
    }
    /**
     * Returns the global tracer provider.
     */
    getTracerProvider() {
        return (0,global_utils/* getGlobal */.mS)(API_NAME) || this._proxyTracerProvider;
    }
    /**
     * Returns a tracer from the global tracer provider.
     */
    getTracer(name, version) {
        return this.getTracerProvider().getTracer(name, version);
    }
    /** Remove the global tracer provider */
    disable() {
        (0,global_utils/* unregisterGlobal */.kv)(API_NAME, diag/* DiagAPI */.K.instance());
        this._proxyTracerProvider = new ProxyTracerProvider();
    }
}
//# sourceMappingURL=trace.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@opentelemetry+api@1.9.1/node_modules/@opentelemetry/api/build/esm/trace-api.js
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
// Split module-level variable definition into separate files to allow
// tree-shaking on each api instance.

/**
 * Entrypoint for trace API
 *
 * @since 1.0.0
 */
const trace = TraceAPI.getInstance();
//# sourceMappingURL=trace-api.js.map

/***/ }),

/***/ 82902:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   l: () => (/* binding */ ROOT_CONTEXT),
/* harmony export */   n: () => (/* binding */ createContextKey)
/* harmony export */ });
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */
/**
 * Get a key to uniquely identify a context value
 *
 * @since 1.0.0
 */
function createContextKey(description) {
    // The specification states that for the same input, multiple calls should
    // return different keys. Due to the nature of the JS dependency management
    // system, this creates problems where multiple versions of some package
    // could hold different keys for the same property.
    //
    // Therefore, we use Symbol.for which returns the same key for the same input.
    return Symbol.for(description);
}
class BaseContext {
    /**
     * Construct a new context which inherits values from an optional parent context.
     *
     * @param parentContext a context from which to inherit values
     */
    constructor(parentContext) {
        // for minification
        const self = this;
        self._currentContext = parentContext ? new Map(parentContext) : new Map();
        self.getValue = (key) => self._currentContext.get(key);
        self.setValue = (key, value) => {
            const context = new BaseContext(self._currentContext);
            context._currentContext.set(key, value);
            return context;
        };
        self.deleteValue = (key) => {
            const context = new BaseContext(self._currentContext);
            context._currentContext.delete(key);
            return context;
        };
    }
}
/**
 * The root context is used as the default parent context when there is no active context
 *
 * @since 1.0.0
 */
const ROOT_CONTEXT = new BaseContext();
//# sourceMappingURL=context.js.map

/***/ }),

/***/ 88335:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   d: () => (/* binding */ NonRecordingSpan)
/* harmony export */ });
/* harmony import */ var _invalid_span_constants__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(66659);
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * The NonRecordingSpan is the default {@link Span} that is used when no Span
 * implementation is available. All operations are no-op including context
 * propagation.
 */
class NonRecordingSpan {
    constructor(spanContext = _invalid_span_constants__WEBPACK_IMPORTED_MODULE_0__/* .INVALID_SPAN_CONTEXT */ .dM) {
        this._spanContext = spanContext;
    }
    // Returns a SpanContext.
    spanContext() {
        return this._spanContext;
    }
    // By default does nothing
    setAttribute(_key, _value) {
        return this;
    }
    // By default does nothing
    setAttributes(_attributes) {
        return this;
    }
    // By default does nothing
    addEvent(_name, _attributes) {
        return this;
    }
    addLink(_link) {
        return this;
    }
    addLinks(_links) {
        return this;
    }
    // By default does nothing
    setStatus(_status) {
        return this;
    }
    // By default does nothing
    updateName(_name) {
        return this;
    }
    // By default does nothing
    end(_endTime) { }
    // isRecording always returns false for NonRecordingSpan.
    isRecording() {
        return false;
    }
    // By default does nothing
    recordException(_exception, _time) { }
}
//# sourceMappingURL=NonRecordingSpan.js.map

/***/ }),

/***/ 92268:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   IP: () => (/* binding */ wrapSpanContext),
/* harmony export */   YA: () => (/* binding */ isSpanContextValid),
/* harmony export */   hX: () => (/* binding */ isValidTraceId)
/* harmony export */ });
/* unused harmony export isValidSpanId */
/* harmony import */ var _invalid_span_constants__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(66659);
/* harmony import */ var _NonRecordingSpan__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(88335);
/*
 * Copyright The OpenTelemetry Authors
 * SPDX-License-Identifier: Apache-2.0
 */


// Valid characters (0-9, a-f, A-F) are marked as 1.
const isHex = new Uint8Array([
    0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
    0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1,
    1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0,
    0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 1, 1, 1, 1,
]);
function isValidHex(id, length) {
    // As of 1.9.0 the id was allowed to be a non-string value,
    // even though it was not possible in the types.
    if (typeof id !== 'string' || id.length !== length)
        return false;
    let r = 0;
    for (let i = 0; i < id.length; i += 4) {
        r +=
            (isHex[id.charCodeAt(i)] | 0) +
                (isHex[id.charCodeAt(i + 1)] | 0) +
                (isHex[id.charCodeAt(i + 2)] | 0) +
                (isHex[id.charCodeAt(i + 3)] | 0);
    }
    return r === length;
}
/**
 * @since 1.0.0
 */
function isValidTraceId(traceId) {
    return isValidHex(traceId, 32) && traceId !== _invalid_span_constants__WEBPACK_IMPORTED_MODULE_0__/* .INVALID_TRACEID */ .RH;
}
/**
 * @since 1.0.0
 */
function isValidSpanId(spanId) {
    return isValidHex(spanId, 16) && spanId !== _invalid_span_constants__WEBPACK_IMPORTED_MODULE_0__/* .INVALID_SPANID */ .w9;
}
/**
 * Returns true if this {@link SpanContext} is valid.
 * @return true if this {@link SpanContext} is valid.
 *
 * @since 1.0.0
 */
function isSpanContextValid(spanContext) {
    return (isValidTraceId(spanContext.traceId) && isValidSpanId(spanContext.spanId));
}
/**
 * Wrap the given {@link SpanContext} in a new non-recording {@link Span}
 *
 * @param spanContext span context to be wrapped
 * @returns a new non-recording {@link Span} with the provided context
 */
function wrapSpanContext(spanContext) {
    return new _NonRecordingSpan__WEBPACK_IMPORTED_MODULE_1__/* .NonRecordingSpan */ .d(spanContext);
}
//# sourceMappingURL=spancontext-utils.js.map

/***/ })

}]);
//# sourceMappingURL=2099-df25dcb607df6574.js.map