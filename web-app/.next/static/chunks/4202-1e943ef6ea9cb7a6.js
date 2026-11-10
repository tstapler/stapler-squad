(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[4202],{

/***/ 2952:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   M: () => (/* binding */ protoInt64)
/* harmony export */ });
/* harmony import */ var _wire_varint_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(43848);
/* provided dependency */ var process = __webpack_require__(18078);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/**
 * Int64Support for the current environment.
 */
const protoInt64 = /*@__PURE__*/ makeInt64Support();
function makeInt64Support() {
    const dv = new DataView(new ArrayBuffer(8));
    // note that Safari 14 implements BigInt, but not the DataView methods
    const ok = typeof BigInt === "function" &&
        typeof dv.getBigInt64 === "function" &&
        typeof dv.getBigUint64 === "function" &&
        typeof dv.setBigInt64 === "function" &&
        typeof dv.setBigUint64 === "function" &&
        (!!globalThis.Deno ||
            typeof process != "object" ||
            typeof process.env != "object" ||
            process.env.BUF_BIGINT_DISABLE !== "1");
    if (ok) {
        const MIN = BigInt("-9223372036854775808");
        const MAX = BigInt("9223372036854775807");
        const UMIN = BigInt("0");
        const UMAX = BigInt("18446744073709551615");
        return {
            zero: BigInt(0),
            supported: true,
            parse(value) {
                const bi = typeof value == "bigint" ? value : BigInt(value);
                if (bi > MAX || bi < MIN) {
                    throw new Error(`invalid int64: ${value}`);
                }
                return bi;
            },
            uParse(value) {
                const bi = typeof value == "bigint" ? value : BigInt(value);
                if (bi > UMAX || bi < UMIN) {
                    throw new Error(`invalid uint64: ${value}`);
                }
                return bi;
            },
            enc(value) {
                dv.setBigInt64(0, this.parse(value), true);
                return {
                    lo: dv.getInt32(0, true),
                    hi: dv.getInt32(4, true),
                };
            },
            uEnc(value) {
                dv.setBigInt64(0, this.uParse(value), true);
                return {
                    lo: dv.getInt32(0, true),
                    hi: dv.getInt32(4, true),
                };
            },
            dec(lo, hi) {
                dv.setInt32(0, lo, true);
                dv.setInt32(4, hi, true);
                return dv.getBigInt64(0, true);
            },
            uDec(lo, hi) {
                dv.setInt32(0, lo, true);
                dv.setInt32(4, hi, true);
                return dv.getBigUint64(0, true);
            },
        };
    }
    return {
        zero: "0",
        supported: false,
        parse(value) {
            if (typeof value != "string") {
                value = value.toString();
            }
            assertInt64String(value);
            return value;
        },
        uParse(value) {
            if (typeof value != "string") {
                value = value.toString();
            }
            assertUInt64String(value);
            return value;
        },
        enc(value) {
            if (typeof value != "string") {
                value = value.toString();
            }
            assertInt64String(value);
            return (0,_wire_varint_js__WEBPACK_IMPORTED_MODULE_0__/* .int64FromString */ .LJ)(value);
        },
        uEnc(value) {
            if (typeof value != "string") {
                value = value.toString();
            }
            assertUInt64String(value);
            return (0,_wire_varint_js__WEBPACK_IMPORTED_MODULE_0__/* .int64FromString */ .LJ)(value);
        },
        dec(lo, hi) {
            return (0,_wire_varint_js__WEBPACK_IMPORTED_MODULE_0__/* .int64ToString */ ._Q)(lo, hi);
        },
        uDec(lo, hi) {
            return (0,_wire_varint_js__WEBPACK_IMPORTED_MODULE_0__/* .uInt64ToString */ .Hc)(lo, hi);
        },
    };
}
function assertInt64String(value) {
    if (!/^-?[0-9]+$/.test(value)) {
        throw new Error("invalid int64: " + value);
    }
}
function assertUInt64String(value) {
    if (!/^[0-9]+$/.test(value)) {
        throw new Error("invalid uint64: " + value);
    }
}


/***/ }),

/***/ 12386:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   X: () => (/* binding */ codeFromString),
/* harmony export */   s: () => (/* binding */ codeToString)
/* harmony export */ });
/* harmony import */ var _code_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(16294);
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/**
 * codeToString returns the string representation of a Code.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function codeToString(value) {
    const name = _code_js__WEBPACK_IMPORTED_MODULE_0__/* .Code */ .C[value];
    if (typeof name != "string") {
        return value.toString();
    }
    return (name[0].toLowerCase() +
        name.substring(1).replace(/[A-Z]/g, (c) => "_" + c.toLowerCase()));
}
let stringToCode;
/**
 * codeFromString parses the string representation of a Code in snake_case.
 * For example, the string "permission_denied" parses into Code.PermissionDenied.
 *
 * If the given string cannot be parsed, the function returns undefined.
 *
 * @private Internal code, does not follow semantic versioning.
 */
function codeFromString(value) {
    if (!stringToCode) {
        stringToCode = {};
        for (const value of Object.values(_code_js__WEBPACK_IMPORTED_MODULE_0__/* .Code */ .C)) {
            if (typeof value == "string") {
                continue;
            }
            stringToCode[codeToString(value)] = value;
        }
    }
    return stringToCode[value];
}


/***/ }),

/***/ 16294:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   C: () => (/* binding */ Code)
/* harmony export */ });
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * Connect represents categories of errors as codes, and each code maps to a
 * specific HTTP status code. The codes and their semantics were chosen to
 * match gRPC. Only the codes below are valid — there are no user-defined
 * codes.
 *
 * See the specification at https://connectrpc.com/docs/protocol#error-codes
 * for details.
 */
var Code;
(function (Code) {
    /**
     * Canceled, usually by the user
     */
    Code[Code["Canceled"] = 1] = "Canceled";
    /**
     * Unknown error
     */
    Code[Code["Unknown"] = 2] = "Unknown";
    /**
     * Argument invalid regardless of system state
     */
    Code[Code["InvalidArgument"] = 3] = "InvalidArgument";
    /**
     * Operation expired, may or may not have completed.
     */
    Code[Code["DeadlineExceeded"] = 4] = "DeadlineExceeded";
    /**
     * Entity not found.
     */
    Code[Code["NotFound"] = 5] = "NotFound";
    /**
     * Entity already exists.
     */
    Code[Code["AlreadyExists"] = 6] = "AlreadyExists";
    /**
     * Operation not authorized.
     */
    Code[Code["PermissionDenied"] = 7] = "PermissionDenied";
    /**
     * Quota exhausted.
     */
    Code[Code["ResourceExhausted"] = 8] = "ResourceExhausted";
    /**
     * Argument invalid in current system state.
     */
    Code[Code["FailedPrecondition"] = 9] = "FailedPrecondition";
    /**
     * Operation aborted.
     */
    Code[Code["Aborted"] = 10] = "Aborted";
    /**
     * Out of bounds, use instead of FailedPrecondition.
     */
    Code[Code["OutOfRange"] = 11] = "OutOfRange";
    /**
     * Operation not implemented or disabled.
     */
    Code[Code["Unimplemented"] = 12] = "Unimplemented";
    /**
     * Internal error, reserved for "serious errors".
     */
    Code[Code["Internal"] = 13] = "Internal";
    /**
     * Unavailable, client should back off and retry.
     */
    Code[Code["Unavailable"] = 14] = "Unavailable";
    /**
     * Unrecoverable data loss or corruption.
     */
    Code[Code["DataLoss"] = 15] = "DataLoss";
    /**
     * Request isn't authenticated.
     */
    Code[Code["Unauthenticated"] = 16] = "Unauthenticated";
})(Code || (Code = {}));


/***/ }),

/***/ 17134:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   M7: () => (/* binding */ scalarZeroValue),
/* harmony export */   MZ: () => (/* binding */ isScalarZeroValue)
/* harmony export */ });
/* unused harmony export scalarEquals */
/* harmony import */ var _proto_int64_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(2952);
/* harmony import */ var _descriptors_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(92806);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.


/**
 * Returns true if both scalar values are equal.
 */
function scalarEquals(type, a, b) {
    if (a === b) {
        // This correctly matches equal values except BYTES and (possibly) 64-bit integers.
        return true;
    }
    // Special case BYTES - we need to compare each byte individually
    if (type == ScalarType.BYTES) {
        if (!(a instanceof Uint8Array) || !(b instanceof Uint8Array)) {
            return false;
        }
        if (a.length !== b.length) {
            return false;
        }
        for (let i = 0; i < a.length; i++) {
            if (a[i] !== b[i]) {
                return false;
            }
        }
        return true;
    }
    // Special case 64-bit integers - we support number, string and bigint representation.
    switch (type) {
        case ScalarType.UINT64:
        case ScalarType.FIXED64:
        case ScalarType.INT64:
        case ScalarType.SFIXED64:
        case ScalarType.SINT64:
            // Loose comparison will match between 0n, 0 and "0".
            return a == b;
    }
    // Anything that hasn't been caught by strict comparison or special cased
    // BYTES and 64-bit integers is not equal.
    return false;
}
/**
 * Returns the zero value for the given scalar type.
 */
function scalarZeroValue(type, longAsString) {
    switch (type) {
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_0__/* .ScalarType */ .L.STRING:
            return "";
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_0__/* .ScalarType */ .L.BOOL:
            return false;
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_0__/* .ScalarType */ .L.DOUBLE:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_0__/* .ScalarType */ .L.FLOAT:
            return 0.0;
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_0__/* .ScalarType */ .L.INT64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_0__/* .ScalarType */ .L.UINT64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_0__/* .ScalarType */ .L.SFIXED64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_0__/* .ScalarType */ .L.FIXED64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_0__/* .ScalarType */ .L.SINT64:
            return (longAsString ? "0" : _proto_int64_js__WEBPACK_IMPORTED_MODULE_1__/* .protoInt64 */ .M.zero);
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_0__/* .ScalarType */ .L.BYTES:
            return new Uint8Array(0);
        default:
            // Handles INT32, UINT32, SINT32, FIXED32, SFIXED32.
            // We do not use individual cases to save a few bytes code size.
            return 0;
    }
}
/**
 * Returns true for a zero-value. For example, an integer has the zero-value `0`,
 * a boolean is `false`, a string is `""`, and bytes is an empty Uint8Array.
 *
 * In proto3, zero-values are not written to the wire, unless the field is
 * optional or repeated.
 */
function isScalarZeroValue(type, value) {
    switch (type) {
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_0__/* .ScalarType */ .L.BOOL:
            return value === false;
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_0__/* .ScalarType */ .L.STRING:
            return value === "";
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_0__/* .ScalarType */ .L.BYTES:
            return value instanceof Uint8Array && !value.byteLength;
        default:
            return value == 0; // Loose comparison matches 0n, 0 and "0"
    }
}


/***/ }),

/***/ 20932:
/***/ ((module) => {

var __dirname = "/";
(function(){var e={229:function(e){var t=e.exports={};var r;var n;function defaultSetTimout(){throw new Error("setTimeout has not been defined")}function defaultClearTimeout(){throw new Error("clearTimeout has not been defined")}(function(){try{if(typeof setTimeout==="function"){r=setTimeout}else{r=defaultSetTimout}}catch(e){r=defaultSetTimout}try{if(typeof clearTimeout==="function"){n=clearTimeout}else{n=defaultClearTimeout}}catch(e){n=defaultClearTimeout}})();function runTimeout(e){if(r===setTimeout){return setTimeout(e,0)}if((r===defaultSetTimout||!r)&&setTimeout){r=setTimeout;return setTimeout(e,0)}try{return r(e,0)}catch(t){try{return r.call(null,e,0)}catch(t){return r.call(this,e,0)}}}function runClearTimeout(e){if(n===clearTimeout){return clearTimeout(e)}if((n===defaultClearTimeout||!n)&&clearTimeout){n=clearTimeout;return clearTimeout(e)}try{return n(e)}catch(t){try{return n.call(null,e)}catch(t){return n.call(this,e)}}}var i=[];var o=false;var u;var a=-1;function cleanUpNextTick(){if(!o||!u){return}o=false;if(u.length){i=u.concat(i)}else{a=-1}if(i.length){drainQueue()}}function drainQueue(){if(o){return}var e=runTimeout(cleanUpNextTick);o=true;var t=i.length;while(t){u=i;i=[];while(++a<t){if(u){u[a].run()}}a=-1;t=i.length}u=null;o=false;runClearTimeout(e)}t.nextTick=function(e){var t=new Array(arguments.length-1);if(arguments.length>1){for(var r=1;r<arguments.length;r++){t[r-1]=arguments[r]}}i.push(new Item(e,t));if(i.length===1&&!o){runTimeout(drainQueue)}};function Item(e,t){this.fun=e;this.array=t}Item.prototype.run=function(){this.fun.apply(null,this.array)};t.title="browser";t.browser=true;t.env={};t.argv=[];t.version="";t.versions={};function noop(){}t.on=noop;t.addListener=noop;t.once=noop;t.off=noop;t.removeListener=noop;t.removeAllListeners=noop;t.emit=noop;t.prependListener=noop;t.prependOnceListener=noop;t.listeners=function(e){return[]};t.binding=function(e){throw new Error("process.binding is not supported")};t.cwd=function(){return"/"};t.chdir=function(e){throw new Error("process.chdir is not supported")};t.umask=function(){return 0}}};var t={};function __nccwpck_require__(r){var n=t[r];if(n!==undefined){return n.exports}var i=t[r]={exports:{}};var o=true;try{e[r](i,i.exports,__nccwpck_require__);o=false}finally{if(o)delete t[r]}return i.exports}if(typeof __nccwpck_require__!=="undefined")__nccwpck_require__.ab=__dirname+"/";var r=__nccwpck_require__(229);module.exports=r})();

/***/ }),

/***/ 22060:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $v: () => (/* binding */ unsafeGet),
/* harmony export */   I4: () => (/* binding */ unsafeLocal),
/* harmony export */   b1: () => (/* binding */ unsafeIsSetExplicit),
/* harmony export */   lA: () => (/* binding */ unsafeSet),
/* harmony export */   mO: () => (/* binding */ unsafeOneofCase),
/* harmony export */   yO: () => (/* binding */ unsafeClear),
/* harmony export */   zB: () => (/* binding */ unsafeIsSet)
/* harmony export */ });
/* harmony import */ var _scalar_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(17134);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// bootstrap-inject google.protobuf.FeatureSet.FieldPresence.IMPLICIT: const $name: FeatureSet_FieldPresence.$localName = $number;
const IMPLICIT = 2;
const unsafeLocal = Symbol.for("reflect unsafe local");
/**
 * Return the selected field of a oneof group.
 *
 * @private
 */
function unsafeOneofCase(
// biome-ignore lint/suspicious/noExplicitAny: `any` is the best choice for dynamic access
target, oneof) {
    const c = target[oneof.localName].case;
    if (c === undefined) {
        return c;
    }
    return oneof.fields.find((f) => f.localName === c);
}
/**
 * Returns true if the field is set.
 *
 * @private
 */
function unsafeIsSet(
// biome-ignore lint/suspicious/noExplicitAny: `any` is the best choice for dynamic access
target, field) {
    const name = field.localName;
    if (field.oneof) {
        return target[field.oneof.localName].case === name;
    }
    if (field.presence != IMPLICIT) {
        // Fields with explicit presence have properties on the prototype chain
        // for default / zero values (except for proto3).
        return (target[name] !== undefined &&
            Object.prototype.hasOwnProperty.call(target, name));
    }
    switch (field.fieldKind) {
        case "list":
            return target[name].length > 0;
        case "map":
            return Object.keys(target[name]).length > 0;
        case "scalar":
            return !(0,_scalar_js__WEBPACK_IMPORTED_MODULE_0__/* .isScalarZeroValue */ .MZ)(field.scalar, target[name]);
        case "enum":
            return target[name] !== field.enum.values[0].number;
    }
    throw new Error("message field with implicit presence");
}
/**
 * Returns true if the field is set, but only for singular fields with explicit
 * presence (proto2).
 *
 * @private
 */
function unsafeIsSetExplicit(target, localName) {
    return (Object.prototype.hasOwnProperty.call(target, localName) &&
        target[localName] !== undefined);
}
/**
 * Return a field value, respecting oneof groups.
 *
 * @private
 */
function unsafeGet(target, field) {
    if (field.oneof) {
        const oneof = target[field.oneof.localName];
        if (oneof.case === field.localName) {
            return oneof.value;
        }
        return undefined;
    }
    return target[field.localName];
}
/**
 * Set a field value, respecting oneof groups.
 *
 * @private
 */
function unsafeSet(target, field, value) {
    if (field.oneof) {
        target[field.oneof.localName] = {
            case: field.localName,
            value: value,
        };
    }
    else {
        target[field.localName] = value;
    }
}
/**
 * Resets the field, so that unsafeIsSet() will return false.
 *
 * @private
 */
function unsafeClear(
// biome-ignore lint/suspicious/noExplicitAny: `any` is the best choice for dynamic access
target, field) {
    const name = field.localName;
    if (field.oneof) {
        const oneofLocalName = field.oneof.localName;
        if (target[oneofLocalName].case === name) {
            target[oneofLocalName] = { case: undefined };
        }
    }
    else if (field.presence != IMPLICIT) {
        // Fields with explicit presence have properties on the prototype chain
        // for default / zero values (except for proto3). By deleting their own
        // property, the field is reset.
        delete target[name];
    }
    else {
        switch (field.fieldKind) {
            case "map":
                target[name] = {};
                break;
            case "list":
                target[name] = [];
                break;
            case "enum":
                target[name] = field.enum.values[0].number;
                break;
            case "scalar":
                target[name] = (0,_scalar_js__WEBPACK_IMPORTED_MODULE_0__/* .scalarZeroValue */ .M7)(field.scalar, field.longAsString);
                break;
        }
    }
}


/***/ }),

/***/ 26811:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   O0: () => (/* binding */ WireType),
/* harmony export */   Qh: () => (/* binding */ INT32_MIN),
/* harmony export */   T: () => (/* binding */ FLOAT32_MIN),
/* harmony export */   V5: () => (/* binding */ BinaryReader),
/* harmony export */   f7: () => (/* binding */ UINT32_MAX),
/* harmony export */   fI: () => (/* binding */ BinaryWriter),
/* harmony export */   go: () => (/* binding */ INT32_MAX),
/* harmony export */   xY: () => (/* binding */ FLOAT32_MAX)
/* harmony export */ });
/* harmony import */ var _varint_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(43848);
/* harmony import */ var _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(2952);
/* harmony import */ var _text_encoding_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(59233);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.



/**
 * Protobuf binary format wire types.
 *
 * A wire type provides just enough information to find the length of the
 * following value.
 *
 * See https://developers.google.com/protocol-buffers/docs/encoding#structure
 */
var WireType;
(function (WireType) {
    /**
     * Used for int32, int64, uint32, uint64, sint32, sint64, bool, enum
     */
    WireType[WireType["Varint"] = 0] = "Varint";
    /**
     * Used for fixed64, sfixed64, double.
     * Always 8 bytes with little-endian byte order.
     */
    WireType[WireType["Bit64"] = 1] = "Bit64";
    /**
     * Used for string, bytes, embedded messages, packed repeated fields
     *
     * Only repeated numeric types (types which use the varint, 32-bit,
     * or 64-bit wire types) can be packed. In proto3, such fields are
     * packed by default.
     */
    WireType[WireType["LengthDelimited"] = 2] = "LengthDelimited";
    /**
     * Start of a tag-delimited aggregate, such as a proto2 group, or a message
     * in editions with message_encoding = DELIMITED.
     */
    WireType[WireType["StartGroup"] = 3] = "StartGroup";
    /**
     * End of a tag-delimited aggregate.
     */
    WireType[WireType["EndGroup"] = 4] = "EndGroup";
    /**
     * Used for fixed32, sfixed32, float.
     * Always 4 bytes with little-endian byte order.
     */
    WireType[WireType["Bit32"] = 5] = "Bit32";
})(WireType || (WireType = {}));
/**
 * Maximum value for a 32-bit floating point value (Protobuf FLOAT).
 */
const FLOAT32_MAX = 3.4028234663852886e38;
/**
 * Minimum value for a 32-bit floating point value (Protobuf FLOAT).
 */
const FLOAT32_MIN = -3.4028234663852886e38;
/**
 * Maximum value for an unsigned 32-bit integer (Protobuf UINT32, FIXED32).
 */
const UINT32_MAX = 0xffffffff;
/**
 * Maximum value for a signed 32-bit integer (Protobuf INT32, SFIXED32, SINT32).
 */
const INT32_MAX = 0x7fffffff;
/**
 * Minimum value for a signed 32-bit integer (Protobuf INT32, SFIXED32, SINT32).
 */
const INT32_MIN = -0x80000000;
class BinaryWriter {
    constructor(encodeUtf8 = (0,_text_encoding_js__WEBPACK_IMPORTED_MODULE_0__/* .getTextEncoding */ .d)().encodeUtf8) {
        this.encodeUtf8 = encodeUtf8;
        /**
         * Previous fork states.
         */
        this.stack = [];
        this.chunks = [];
        this.buf = [];
    }
    /**
     * Return all bytes written and reset this writer.
     */
    finish() {
        if (this.buf.length) {
            this.chunks.push(new Uint8Array(this.buf)); // flush the buffer
            this.buf = [];
        }
        let len = 0;
        for (let i = 0; i < this.chunks.length; i++)
            len += this.chunks[i].length;
        let bytes = new Uint8Array(len);
        let offset = 0;
        for (let i = 0; i < this.chunks.length; i++) {
            bytes.set(this.chunks[i], offset);
            offset += this.chunks[i].length;
        }
        this.chunks = [];
        return bytes;
    }
    /**
     * Start a new fork for length-delimited data like a message
     * or a packed repeated field.
     *
     * Must be joined later with `join()`.
     */
    fork() {
        this.stack.push({ chunks: this.chunks, buf: this.buf });
        this.chunks = [];
        this.buf = [];
        return this;
    }
    /**
     * Join the last fork. Write its length and bytes, then
     * return to the previous state.
     */
    join() {
        // get chunk of fork
        let chunk = this.finish();
        // restore previous state
        let prev = this.stack.pop();
        if (!prev)
            throw new Error("invalid state, fork stack empty");
        this.chunks = prev.chunks;
        this.buf = prev.buf;
        // write length of chunk as varint
        this.uint32(chunk.byteLength);
        return this.raw(chunk);
    }
    /**
     * Writes a tag (field number and wire type).
     *
     * Equivalent to `uint32( (fieldNo << 3 | type) >>> 0 )`.
     *
     * Generated code should compute the tag ahead of time and call `uint32()`.
     */
    tag(fieldNo, type) {
        return this.uint32(((fieldNo << 3) | type) >>> 0);
    }
    /**
     * Write a chunk of raw bytes.
     */
    raw(chunk) {
        if (this.buf.length) {
            this.chunks.push(new Uint8Array(this.buf));
            this.buf = [];
        }
        this.chunks.push(chunk);
        return this;
    }
    /**
     * Write a `uint32` value, an unsigned 32 bit varint.
     */
    uint32(value) {
        assertUInt32(value);
        // write value as varint 32, inlined for speed
        while (value > 0x7f) {
            this.buf.push((value & 0x7f) | 0x80);
            value = value >>> 7;
        }
        this.buf.push(value);
        return this;
    }
    /**
     * Write a `int32` value, a signed 32 bit varint.
     */
    int32(value) {
        assertInt32(value);
        (0,_varint_js__WEBPACK_IMPORTED_MODULE_1__/* .varint32write */ .f7)(value, this.buf);
        return this;
    }
    /**
     * Write a `bool` value, a variant.
     */
    bool(value) {
        this.buf.push(value ? 1 : 0);
        return this;
    }
    /**
     * Write a `bytes` value, length-delimited arbitrary data.
     */
    bytes(value) {
        this.uint32(value.byteLength); // write length of chunk as varint
        return this.raw(value);
    }
    /**
     * Write a `string` value, length-delimited data converted to UTF-8 text.
     */
    string(value) {
        let chunk = this.encodeUtf8(value);
        this.uint32(chunk.byteLength); // write length of chunk as varint
        return this.raw(chunk);
    }
    /**
     * Write a `float` value, 32-bit floating point number.
     */
    float(value) {
        assertFloat32(value);
        let chunk = new Uint8Array(4);
        new DataView(chunk.buffer).setFloat32(0, value, true);
        return this.raw(chunk);
    }
    /**
     * Write a `double` value, a 64-bit floating point number.
     */
    double(value) {
        let chunk = new Uint8Array(8);
        new DataView(chunk.buffer).setFloat64(0, value, true);
        return this.raw(chunk);
    }
    /**
     * Write a `fixed32` value, an unsigned, fixed-length 32-bit integer.
     */
    fixed32(value) {
        assertUInt32(value);
        let chunk = new Uint8Array(4);
        new DataView(chunk.buffer).setUint32(0, value, true);
        return this.raw(chunk);
    }
    /**
     * Write a `sfixed32` value, a signed, fixed-length 32-bit integer.
     */
    sfixed32(value) {
        assertInt32(value);
        let chunk = new Uint8Array(4);
        new DataView(chunk.buffer).setInt32(0, value, true);
        return this.raw(chunk);
    }
    /**
     * Write a `sint32` value, a signed, zigzag-encoded 32-bit varint.
     */
    sint32(value) {
        assertInt32(value);
        // zigzag encode
        value = ((value << 1) ^ (value >> 31)) >>> 0;
        (0,_varint_js__WEBPACK_IMPORTED_MODULE_1__/* .varint32write */ .f7)(value, this.buf);
        return this;
    }
    /**
     * Write a `fixed64` value, a signed, fixed-length 64-bit integer.
     */
    sfixed64(value) {
        let chunk = new Uint8Array(8), view = new DataView(chunk.buffer), tc = _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__/* .protoInt64 */ .M.enc(value);
        view.setInt32(0, tc.lo, true);
        view.setInt32(4, tc.hi, true);
        return this.raw(chunk);
    }
    /**
     * Write a `fixed64` value, an unsigned, fixed-length 64 bit integer.
     */
    fixed64(value) {
        let chunk = new Uint8Array(8), view = new DataView(chunk.buffer), tc = _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__/* .protoInt64 */ .M.uEnc(value);
        view.setInt32(0, tc.lo, true);
        view.setInt32(4, tc.hi, true);
        return this.raw(chunk);
    }
    /**
     * Write a `int64` value, a signed 64-bit varint.
     */
    int64(value) {
        let tc = _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__/* .protoInt64 */ .M.enc(value);
        (0,_varint_js__WEBPACK_IMPORTED_MODULE_1__/* .varint64write */ .cV)(tc.lo, tc.hi, this.buf);
        return this;
    }
    /**
     * Write a `sint64` value, a signed, zig-zag-encoded 64-bit varint.
     */
    sint64(value) {
        const tc = _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__/* .protoInt64 */ .M.enc(value), 
        // zigzag encode
        sign = tc.hi >> 31, lo = (tc.lo << 1) ^ sign, hi = ((tc.hi << 1) | (tc.lo >>> 31)) ^ sign;
        (0,_varint_js__WEBPACK_IMPORTED_MODULE_1__/* .varint64write */ .cV)(lo, hi, this.buf);
        return this;
    }
    /**
     * Write a `uint64` value, an unsigned 64-bit varint.
     */
    uint64(value) {
        const tc = _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__/* .protoInt64 */ .M.uEnc(value);
        (0,_varint_js__WEBPACK_IMPORTED_MODULE_1__/* .varint64write */ .cV)(tc.lo, tc.hi, this.buf);
        return this;
    }
}
class BinaryReader {
    constructor(buf, decodeUtf8 = (0,_text_encoding_js__WEBPACK_IMPORTED_MODULE_0__/* .getTextEncoding */ .d)().decodeUtf8) {
        this.decodeUtf8 = decodeUtf8;
        this.varint64 = _varint_js__WEBPACK_IMPORTED_MODULE_1__/* .varint64read */ .ls; // dirty cast for `this`
        /**
         * Read a `uint32` field, an unsigned 32 bit varint.
         */
        this.uint32 = _varint_js__WEBPACK_IMPORTED_MODULE_1__/* .varint32read */ .Gn;
        this.buf = buf;
        this.len = buf.length;
        this.pos = 0;
        this.view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    }
    /**
     * Reads a tag - field number and wire type.
     */
    tag() {
        let tag = this.uint32(), fieldNo = tag >>> 3, wireType = tag & 7;
        if (fieldNo <= 0 || wireType < 0 || wireType > 5)
            throw new Error("illegal tag: field no " + fieldNo + " wire type " + wireType);
        return [fieldNo, wireType];
    }
    /**
     * Skip one element and return the skipped data.
     *
     * When skipping StartGroup, provide the tags field number to check for
     * matching field number in the EndGroup tag.
     */
    skip(wireType, fieldNo) {
        let start = this.pos;
        switch (wireType) {
            case WireType.Varint:
                while (this.buf[this.pos++] & 0x80) {
                    // ignore
                }
                break;
            // @ts-ignore TS7029: Fallthrough case in switch -- ignore instead of expect-error for compiler settings without noFallthroughCasesInSwitch: true
            case WireType.Bit64:
                this.pos += 4;
            case WireType.Bit32:
                this.pos += 4;
                break;
            case WireType.LengthDelimited:
                let len = this.uint32();
                this.pos += len;
                break;
            case WireType.StartGroup:
                for (;;) {
                    const [fn, wt] = this.tag();
                    if (wt === WireType.EndGroup) {
                        if (fieldNo !== undefined && fn !== fieldNo) {
                            throw new Error("invalid end group tag");
                        }
                        break;
                    }
                    this.skip(wt, fn);
                }
                break;
            default:
                throw new Error("cant skip wire type " + wireType);
        }
        this.assertBounds();
        return this.buf.subarray(start, this.pos);
    }
    /**
     * Throws error if position in byte array is out of range.
     */
    assertBounds() {
        if (this.pos > this.len)
            throw new RangeError("premature EOF");
    }
    /**
     * Read a `int32` field, a signed 32 bit varint.
     */
    int32() {
        return this.uint32() | 0;
    }
    /**
     * Read a `sint32` field, a signed, zigzag-encoded 32-bit varint.
     */
    sint32() {
        let zze = this.uint32();
        // decode zigzag
        return (zze >>> 1) ^ -(zze & 1);
    }
    /**
     * Read a `int64` field, a signed 64-bit varint.
     */
    int64() {
        return _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__/* .protoInt64 */ .M.dec(...this.varint64());
    }
    /**
     * Read a `uint64` field, an unsigned 64-bit varint.
     */
    uint64() {
        return _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__/* .protoInt64 */ .M.uDec(...this.varint64());
    }
    /**
     * Read a `sint64` field, a signed, zig-zag-encoded 64-bit varint.
     */
    sint64() {
        let [lo, hi] = this.varint64();
        // decode zig zag
        let s = -(lo & 1);
        lo = ((lo >>> 1) | ((hi & 1) << 31)) ^ s;
        hi = (hi >>> 1) ^ s;
        return _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__/* .protoInt64 */ .M.dec(lo, hi);
    }
    /**
     * Read a `bool` field, a variant.
     */
    bool() {
        let [lo, hi] = this.varint64();
        return lo !== 0 || hi !== 0;
    }
    /**
     * Read a `fixed32` field, an unsigned, fixed-length 32-bit integer.
     */
    fixed32() {
        // biome-ignore lint/suspicious/noAssignInExpressions: no
        return this.view.getUint32((this.pos += 4) - 4, true);
    }
    /**
     * Read a `sfixed32` field, a signed, fixed-length 32-bit integer.
     */
    sfixed32() {
        // biome-ignore lint/suspicious/noAssignInExpressions: no
        return this.view.getInt32((this.pos += 4) - 4, true);
    }
    /**
     * Read a `fixed64` field, an unsigned, fixed-length 64 bit integer.
     */
    fixed64() {
        return _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__/* .protoInt64 */ .M.uDec(this.sfixed32(), this.sfixed32());
    }
    /**
     * Read a `fixed64` field, a signed, fixed-length 64-bit integer.
     */
    sfixed64() {
        return _proto_int64_js__WEBPACK_IMPORTED_MODULE_2__/* .protoInt64 */ .M.dec(this.sfixed32(), this.sfixed32());
    }
    /**
     * Read a `float` field, 32-bit floating point number.
     */
    float() {
        // biome-ignore lint/suspicious/noAssignInExpressions: no
        return this.view.getFloat32((this.pos += 4) - 4, true);
    }
    /**
     * Read a `double` field, a 64-bit floating point number.
     */
    double() {
        // biome-ignore lint/suspicious/noAssignInExpressions: no
        return this.view.getFloat64((this.pos += 8) - 8, true);
    }
    /**
     * Read a `bytes` field, length-delimited arbitrary data.
     */
    bytes() {
        let len = this.uint32(), start = this.pos;
        this.pos += len;
        this.assertBounds();
        return this.buf.subarray(start, start + len);
    }
    /**
     * Read a `string` field, length-delimited data converted to UTF-8 text.
     */
    string() {
        return this.decodeUtf8(this.bytes());
    }
}
/**
 * Assert a valid signed protobuf 32-bit integer as a number or string.
 */
function assertInt32(arg) {
    if (typeof arg == "string") {
        arg = Number(arg);
    }
    else if (typeof arg != "number") {
        throw new Error("invalid int32: " + typeof arg);
    }
    if (!Number.isInteger(arg) ||
        arg > INT32_MAX ||
        arg < INT32_MIN)
        throw new Error("invalid int32: " + arg);
}
/**
 * Assert a valid unsigned protobuf 32-bit integer as a number or string.
 */
function assertUInt32(arg) {
    if (typeof arg == "string") {
        arg = Number(arg);
    }
    else if (typeof arg != "number") {
        throw new Error("invalid uint32: " + typeof arg);
    }
    if (!Number.isInteger(arg) ||
        arg > UINT32_MAX ||
        arg < 0)
        throw new Error("invalid uint32: " + arg);
}
/**
 * Assert a valid protobuf float value as a number or string.
 */
function assertFloat32(arg) {
    if (typeof arg == "string") {
        const o = arg;
        arg = Number(arg);
        if (Number.isNaN(arg) && o !== "NaN") {
            throw new Error("invalid float32: " + o);
        }
    }
    else if (typeof arg != "number") {
        throw new Error("invalid float32: " + typeof arg);
    }
    if (Number.isFinite(arg) &&
        (arg > FLOAT32_MAX || arg < FLOAT32_MIN))
        throw new Error("invalid float32: " + arg);
}


/***/ }),

/***/ 34123:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Y: () => (/* binding */ isWrapper),
/* harmony export */   l: () => (/* binding */ isWrapperDesc)
/* harmony export */ });
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
function isWrapper(arg) {
    return isWrapperTypeName(arg.$typeName);
}
function isWrapperDesc(messageDesc) {
    const f = messageDesc.fields[0];
    return (isWrapperTypeName(messageDesc.typeName) &&
        f !== undefined &&
        f.fieldKind == "scalar" &&
        f.name == "value" &&
        f.number == 1);
}
function isWrapperTypeName(name) {
    return (name.startsWith("google.protobuf.") &&
        [
            "DoubleValue",
            "FloatValue",
            "Int64Value",
            "UInt64Value",
            "Int32Value",
            "UInt32Value",
            "BoolValue",
            "StringValue",
            "BytesValue",
        ].includes(name.substring(16)));
}


/***/ }),

/***/ 36170:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   K: () => (/* binding */ isMessage)
/* harmony export */ });
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * Determine whether the given `arg` is a message.
 * If `desc` is set, determine whether `arg` is this specific message.
 */
function isMessage(arg, schema) {
    const isMessage = arg !== null &&
        typeof arg == "object" &&
        "$typeName" in arg &&
        typeof arg.$typeName == "string";
    if (!isMessage) {
        return false;
    }
    if (schema === undefined) {
        return true;
    }
    return schema.typeName === arg.$typeName;
}


/***/ }),

/***/ 36843:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";

var _global_process, _global_process1;
module.exports = ((_global_process = __webpack_require__.g.process) == null ? void 0 : _global_process.env) && typeof ((_global_process1 = __webpack_require__.g.process) == null ? void 0 : _global_process1.env) === 'object' ? __webpack_require__.g.process : __webpack_require__(20932);

//# sourceMappingURL=process.js.map

/***/ }),

/***/ 43848:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Gn: () => (/* binding */ varint32read),
/* harmony export */   Hc: () => (/* binding */ uInt64ToString),
/* harmony export */   LJ: () => (/* binding */ int64FromString),
/* harmony export */   _Q: () => (/* binding */ int64ToString),
/* harmony export */   cV: () => (/* binding */ varint64write),
/* harmony export */   f7: () => (/* binding */ varint32write),
/* harmony export */   ls: () => (/* binding */ varint64read)
/* harmony export */ });
// Copyright 2008 Google Inc.  All rights reserved.
//
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are
// met:
//
// * Redistributions of source code must retain the above copyright
// notice, this list of conditions and the following disclaimer.
// * Redistributions in binary form must reproduce the above
// copyright notice, this list of conditions and the following disclaimer
// in the documentation and/or other materials provided with the
// distribution.
// * Neither the name of Google Inc. nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
// "AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
// LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
// A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
// OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
// SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
// LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
// DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
// THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
// (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
//
// Code generated by the Protocol Buffer compiler is owned by the owner
// of the input file used when generating it.  This code is not
// standalone and requires a support library to be linked with it.  This
// support library is itself covered by the above license.
/**
 * Read a 64 bit varint as two JS numbers.
 *
 * Returns tuple:
 * [0]: low bits
 * [1]: high bits
 *
 * Copyright 2008 Google Inc.  All rights reserved.
 *
 * See https://github.com/protocolbuffers/protobuf/blob/8a71927d74a4ce34efe2d8769fda198f52d20d12/js/experimental/runtime/kernel/buffer_decoder.js#L175
 */
function varint64read() {
    let lowBits = 0;
    let highBits = 0;
    for (let shift = 0; shift < 28; shift += 7) {
        let b = this.buf[this.pos++];
        lowBits |= (b & 0x7f) << shift;
        if ((b & 0x80) == 0) {
            this.assertBounds();
            return [lowBits, highBits];
        }
    }
    let middleByte = this.buf[this.pos++];
    // last four bits of the first 32 bit number
    lowBits |= (middleByte & 0x0f) << 28;
    // 3 upper bits are part of the next 32 bit number
    highBits = (middleByte & 0x70) >> 4;
    if ((middleByte & 0x80) == 0) {
        this.assertBounds();
        return [lowBits, highBits];
    }
    for (let shift = 3; shift <= 31; shift += 7) {
        let b = this.buf[this.pos++];
        highBits |= (b & 0x7f) << shift;
        if ((b & 0x80) == 0) {
            this.assertBounds();
            return [lowBits, highBits];
        }
    }
    throw new Error("invalid varint");
}
/**
 * Write a 64 bit varint, given as two JS numbers, to the given bytes array.
 *
 * Copyright 2008 Google Inc.  All rights reserved.
 *
 * See https://github.com/protocolbuffers/protobuf/blob/8a71927d74a4ce34efe2d8769fda198f52d20d12/js/experimental/runtime/kernel/writer.js#L344
 */
function varint64write(lo, hi, bytes) {
    for (let i = 0; i < 28; i = i + 7) {
        const shift = lo >>> i;
        const hasNext = !(shift >>> 7 == 0 && hi == 0);
        const byte = (hasNext ? shift | 0x80 : shift) & 0xff;
        bytes.push(byte);
        if (!hasNext) {
            return;
        }
    }
    const splitBits = ((lo >>> 28) & 0x0f) | ((hi & 0x07) << 4);
    const hasMoreBits = !(hi >> 3 == 0);
    bytes.push((hasMoreBits ? splitBits | 0x80 : splitBits) & 0xff);
    if (!hasMoreBits) {
        return;
    }
    for (let i = 3; i < 31; i = i + 7) {
        const shift = hi >>> i;
        const hasNext = !(shift >>> 7 == 0);
        const byte = (hasNext ? shift | 0x80 : shift) & 0xff;
        bytes.push(byte);
        if (!hasNext) {
            return;
        }
    }
    bytes.push((hi >>> 31) & 0x01);
}
// constants for binary math
const TWO_PWR_32_DBL = 0x100000000;
/**
 * Parse decimal string of 64 bit integer value as two JS numbers.
 *
 * Copyright 2008 Google Inc.  All rights reserved.
 *
 * See https://github.com/protocolbuffers/protobuf-javascript/blob/a428c58273abad07c66071d9753bc4d1289de426/experimental/runtime/int64.js#L10
 */
function int64FromString(dec) {
    // Check for minus sign.
    const minus = dec[0] === "-";
    if (minus) {
        dec = dec.slice(1);
    }
    // Work 6 decimal digits at a time, acting like we're converting base 1e6
    // digits to binary. This is safe to do with floating point math because
    // Number.isSafeInteger(ALL_32_BITS * 1e6) == true.
    const base = 1e6;
    let lowBits = 0;
    let highBits = 0;
    function add1e6digit(begin, end) {
        // Note: Number('') is 0.
        const digit1e6 = Number(dec.slice(begin, end));
        highBits *= base;
        lowBits = lowBits * base + digit1e6;
        // Carry bits from lowBits to
        if (lowBits >= TWO_PWR_32_DBL) {
            highBits = highBits + ((lowBits / TWO_PWR_32_DBL) | 0);
            lowBits = lowBits % TWO_PWR_32_DBL;
        }
    }
    add1e6digit(-24, -18);
    add1e6digit(-18, -12);
    add1e6digit(-12, -6);
    add1e6digit(-6);
    return minus ? negate(lowBits, highBits) : newBits(lowBits, highBits);
}
/**
 * Losslessly converts a 64-bit signed integer in 32:32 split representation
 * into a decimal string.
 *
 * Copyright 2008 Google Inc.  All rights reserved.
 *
 * See https://github.com/protocolbuffers/protobuf-javascript/blob/a428c58273abad07c66071d9753bc4d1289de426/experimental/runtime/int64.js#L10
 */
function int64ToString(lo, hi) {
    let bits = newBits(lo, hi);
    // If we're treating the input as a signed value and the high bit is set, do
    // a manual two's complement conversion before the decimal conversion.
    const negative = bits.hi & 0x80000000;
    if (negative) {
        bits = negate(bits.lo, bits.hi);
    }
    const result = uInt64ToString(bits.lo, bits.hi);
    return negative ? "-" + result : result;
}
/**
 * Losslessly converts a 64-bit unsigned integer in 32:32 split representation
 * into a decimal string.
 *
 * Copyright 2008 Google Inc.  All rights reserved.
 *
 * See https://github.com/protocolbuffers/protobuf-javascript/blob/a428c58273abad07c66071d9753bc4d1289de426/experimental/runtime/int64.js#L10
 */
function uInt64ToString(lo, hi) {
    ({ lo, hi } = toUnsigned(lo, hi));
    // Skip the expensive conversion if the number is small enough to use the
    // built-in conversions.
    // Number.MAX_SAFE_INTEGER = 0x001FFFFF FFFFFFFF, thus any number with
    // highBits <= 0x1FFFFF can be safely expressed with a double and retain
    // integer precision.
    // Proven by: Number.isSafeInteger(0x1FFFFF * 2**32 + 0xFFFFFFFF) == true.
    if (hi <= 0x1fffff) {
        return String(TWO_PWR_32_DBL * hi + lo);
    }
    // What this code is doing is essentially converting the input number from
    // base-2 to base-1e7, which allows us to represent the 64-bit range with
    // only 3 (very large) digits. Those digits are then trivial to convert to
    // a base-10 string.
    // The magic numbers used here are -
    // 2^24 = 16777216 = (1,6777216) in base-1e7.
    // 2^48 = 281474976710656 = (2,8147497,6710656) in base-1e7.
    // Split 32:32 representation into 16:24:24 representation so our
    // intermediate digits don't overflow.
    const low = lo & 0xffffff;
    const mid = ((lo >>> 24) | (hi << 8)) & 0xffffff;
    const high = (hi >> 16) & 0xffff;
    // Assemble our three base-1e7 digits, ignoring carries. The maximum
    // value in a digit at this step is representable as a 48-bit integer, which
    // can be stored in a 64-bit floating point number.
    let digitA = low + mid * 6777216 + high * 6710656;
    let digitB = mid + high * 8147497;
    let digitC = high * 2;
    // Apply carries from A to B and from B to C.
    const base = 10000000;
    if (digitA >= base) {
        digitB += Math.floor(digitA / base);
        digitA %= base;
    }
    if (digitB >= base) {
        digitC += Math.floor(digitB / base);
        digitB %= base;
    }
    // If digitC is 0, then we should have returned in the trivial code path
    // at the top for non-safe integers. Given this, we can assume both digitB
    // and digitA need leading zeros.
    return (digitC.toString() +
        decimalFrom1e7WithLeadingZeros(digitB) +
        decimalFrom1e7WithLeadingZeros(digitA));
}
function toUnsigned(lo, hi) {
    return { lo: lo >>> 0, hi: hi >>> 0 };
}
function newBits(lo, hi) {
    return { lo: lo | 0, hi: hi | 0 };
}
/**
 * Returns two's compliment negation of input.
 * @see https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Operators/Bitwise_Operators#Signed_32-bit_integers
 */
function negate(lowBits, highBits) {
    highBits = ~highBits;
    if (lowBits) {
        lowBits = ~lowBits + 1;
    }
    else {
        // If lowBits is 0, then bitwise-not is 0xFFFFFFFF,
        // adding 1 to that, results in 0x100000000, which leaves
        // the low bits 0x0 and simply adds one to the high bits.
        highBits += 1;
    }
    return newBits(lowBits, highBits);
}
/**
 * Returns decimal representation of digit1e7 with leading zeros.
 */
const decimalFrom1e7WithLeadingZeros = (digit1e7) => {
    const partial = String(digit1e7);
    return "0000000".slice(partial.length) + partial;
};
/**
 * Write a 32 bit varint, signed or unsigned. Same as `varint64write(0, value, bytes)`
 *
 * Copyright 2008 Google Inc.  All rights reserved.
 *
 * See https://github.com/protocolbuffers/protobuf/blob/1b18833f4f2a2f681f4e4a25cdf3b0a43115ec26/js/binary/encoder.js#L144
 */
function varint32write(value, bytes) {
    if (value >= 0) {
        // write value as varint 32
        while (value > 0x7f) {
            bytes.push((value & 0x7f) | 0x80);
            value = value >>> 7;
        }
        bytes.push(value);
    }
    else {
        for (let i = 0; i < 9; i++) {
            bytes.push((value & 127) | 128);
            value = value >> 7;
        }
        bytes.push(1);
    }
}
/**
 * Read an unsigned 32 bit varint.
 *
 * See https://github.com/protocolbuffers/protobuf/blob/8a71927d74a4ce34efe2d8769fda198f52d20d12/js/experimental/runtime/kernel/buffer_decoder.js#L220
 */
function varint32read() {
    let b = this.buf[this.pos++];
    let result = b & 0x7f;
    if ((b & 0x80) == 0) {
        this.assertBounds();
        return result;
    }
    b = this.buf[this.pos++];
    result |= (b & 0x7f) << 7;
    if ((b & 0x80) == 0) {
        this.assertBounds();
        return result;
    }
    b = this.buf[this.pos++];
    result |= (b & 0x7f) << 14;
    if ((b & 0x80) == 0) {
        this.assertBounds();
        return result;
    }
    b = this.buf[this.pos++];
    result |= (b & 0x7f) << 21;
    if ((b & 0x80) == 0) {
        this.assertBounds();
        return result;
    }
    // Extract only last 4 bits
    b = this.buf[this.pos++];
    result |= (b & 0x0f) << 28;
    for (let readBytes = 5; (b & 0x80) !== 0 && readBytes < 10; readBytes++)
        b = this.buf[this.pos++];
    if ((b & 0x80) != 0)
        throw new Error("invalid varint");
    this.assertBounds();
    // Result can have 32 bits, convert it to unsigned
    return result >>> 0;
}


/***/ }),

/***/ 45394:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $L: () => (/* binding */ formatVal),
/* harmony export */   MJ: () => (/* binding */ checkListItem),
/* harmony export */   PZ: () => (/* binding */ checkField),
/* harmony export */   xI: () => (/* binding */ checkMapEntry)
/* harmony export */ });
/* harmony import */ var _descriptors_js__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(92806);
/* harmony import */ var _is_message_js__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(36170);
/* harmony import */ var _error_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(64444);
/* harmony import */ var _guard_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(71035);
/* harmony import */ var _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(26811);
/* harmony import */ var _wire_text_encoding_js__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(59233);
/* harmony import */ var _proto_int64_js__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(2952);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.







/**
 * Check whether the given field value is valid for the reflect API.
 */
function checkField(field, value) {
    const check = field.fieldKind == "list"
        ? (0,_guard_js__WEBPACK_IMPORTED_MODULE_0__/* .isReflectList */ .sG)(value, field)
        : field.fieldKind == "map"
            ? (0,_guard_js__WEBPACK_IMPORTED_MODULE_0__/* .isReflectMap */ .G6)(value, field)
            : checkSingular(field, value);
    if (check === true) {
        return undefined;
    }
    let reason;
    switch (field.fieldKind) {
        case "list":
            reason = `expected ${formatReflectList(field)}, got ${formatVal(value)}`;
            break;
        case "map":
            reason = `expected ${formatReflectMap(field)}, got ${formatVal(value)}`;
            break;
        default: {
            reason = reasonSingular(field, value, check);
        }
    }
    return new _error_js__WEBPACK_IMPORTED_MODULE_1__/* .FieldError */ .b(field, reason);
}
/**
 * Check whether the given list item is valid for the reflect API.
 */
function checkListItem(field, index, value) {
    const check = checkSingular(field, value);
    if (check !== true) {
        return new _error_js__WEBPACK_IMPORTED_MODULE_1__/* .FieldError */ .b(field, `list item #${index + 1}: ${reasonSingular(field, value, check)}`);
    }
    return undefined;
}
/**
 * Check whether the given map key and value are valid for the reflect API.
 */
function checkMapEntry(field, key, value) {
    const checkKey = checkScalarValue(key, field.mapKey);
    if (checkKey !== true) {
        return new _error_js__WEBPACK_IMPORTED_MODULE_1__/* .FieldError */ .b(field, `invalid map key: ${reasonSingular({ scalar: field.mapKey }, key, checkKey)}`);
    }
    const checkVal = checkSingular(field, value);
    if (checkVal !== true) {
        return new _error_js__WEBPACK_IMPORTED_MODULE_1__/* .FieldError */ .b(field, `map entry ${formatVal(key)}: ${reasonSingular(field, value, checkVal)}`);
    }
    return undefined;
}
function checkSingular(field, value) {
    if (field.scalar !== undefined) {
        return checkScalarValue(value, field.scalar);
    }
    if (field.enum !== undefined) {
        if (field.enum.open) {
            return Number.isInteger(value);
        }
        return field.enum.values.some((v) => v.number === value);
    }
    return (0,_guard_js__WEBPACK_IMPORTED_MODULE_0__/* .isReflectMessage */ .LT)(value, field.message);
}
function checkScalarValue(value, scalar) {
    switch (scalar) {
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.DOUBLE:
            return typeof value == "number";
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FLOAT:
            if (typeof value != "number") {
                return false;
            }
            if (Number.isNaN(value) || !Number.isFinite(value)) {
                return true;
            }
            if (value > _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_3__/* .FLOAT32_MAX */ .xY || value < _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_3__/* .FLOAT32_MIN */ .T) {
                return `${value.toFixed()} out of range`;
            }
            return true;
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.INT32:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SFIXED32:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SINT32:
            // signed
            if (typeof value !== "number" || !Number.isInteger(value)) {
                return false;
            }
            if (value > _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_3__/* .INT32_MAX */ .go || value < _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_3__/* .INT32_MIN */ .Qh) {
                return `${value.toFixed()} out of range`;
            }
            return true;
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FIXED32:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.UINT32:
            // unsigned
            if (typeof value !== "number" || !Number.isInteger(value)) {
                return false;
            }
            if (value > _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_3__/* .UINT32_MAX */ .f7 || value < 0) {
                return `${value.toFixed()} out of range`;
            }
            return true;
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.BOOL:
            return typeof value == "boolean";
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.STRING:
            if (typeof value != "string") {
                return false;
            }
            return (0,_wire_text_encoding_js__WEBPACK_IMPORTED_MODULE_4__/* .getTextEncoding */ .d)().checkUtf8(value) || "invalid UTF8";
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.BYTES:
            return value instanceof Uint8Array;
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.INT64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SFIXED64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SINT64:
            // signed
            if (typeof value == "bigint" ||
                typeof value == "number" ||
                (typeof value == "string" && value.length > 0)) {
                try {
                    _proto_int64_js__WEBPACK_IMPORTED_MODULE_5__/* .protoInt64 */ .M.parse(value);
                    return true;
                }
                catch (_) {
                    return `${value} out of range`;
                }
            }
            return false;
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FIXED64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.UINT64:
            // unsigned
            if (typeof value == "bigint" ||
                typeof value == "number" ||
                (typeof value == "string" && value.length > 0)) {
                try {
                    _proto_int64_js__WEBPACK_IMPORTED_MODULE_5__/* .protoInt64 */ .M.uParse(value);
                    return true;
                }
                catch (_) {
                    return `${value} out of range`;
                }
            }
            return false;
    }
}
function reasonSingular(field, val, details) {
    details =
        typeof details == "string" ? `: ${details}` : `, got ${formatVal(val)}`;
    if (field.scalar !== undefined) {
        return `expected ${scalarTypeDescription(field.scalar)}` + details;
    }
    if (field.enum !== undefined) {
        return `expected ${field.enum.toString()}` + details;
    }
    return `expected ${formatReflectMessage(field.message)}` + details;
}
function formatVal(val) {
    switch (typeof val) {
        case "object":
            if (val === null) {
                return "null";
            }
            if (val instanceof Uint8Array) {
                return `Uint8Array(${val.length})`;
            }
            if (Array.isArray(val)) {
                return `Array(${val.length})`;
            }
            if ((0,_guard_js__WEBPACK_IMPORTED_MODULE_0__/* .isReflectList */ .sG)(val)) {
                return formatReflectList(val.field());
            }
            if ((0,_guard_js__WEBPACK_IMPORTED_MODULE_0__/* .isReflectMap */ .G6)(val)) {
                return formatReflectMap(val.field());
            }
            if ((0,_guard_js__WEBPACK_IMPORTED_MODULE_0__/* .isReflectMessage */ .LT)(val)) {
                return formatReflectMessage(val.desc);
            }
            if ((0,_is_message_js__WEBPACK_IMPORTED_MODULE_6__/* .isMessage */ .K)(val)) {
                return `message ${val.$typeName}`;
            }
            return "object";
        case "string":
            return val.length > 30 ? "string" : `"${val.split('"').join('\\"')}"`;
        case "boolean":
            return String(val);
        case "number":
            return String(val);
        case "bigint":
            return String(val) + "n";
        default:
            // "symbol" | "undefined" | "object" | "function"
            return typeof val;
    }
}
function formatReflectMessage(desc) {
    return `ReflectMessage (${desc.typeName})`;
}
function formatReflectList(field) {
    switch (field.listKind) {
        case "message":
            return `ReflectList (${field.message.toString()})`;
        case "enum":
            return `ReflectList (${field.enum.toString()})`;
        case "scalar":
            return `ReflectList (${_descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L[field.scalar]})`;
    }
}
function formatReflectMap(field) {
    switch (field.mapKind) {
        case "message":
            return `ReflectMap (${_descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L[field.mapKey]}, ${field.message.toString()})`;
        case "enum":
            return `ReflectMap (${_descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L[field.mapKey]}, ${field.enum.toString()})`;
        case "scalar":
            return `ReflectMap (${_descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L[field.mapKey]}, ${_descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L[field.scalar]})`;
    }
}
function scalarTypeDescription(scalar) {
    switch (scalar) {
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.STRING:
            return "string";
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.BOOL:
            return "boolean";
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.INT64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SINT64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SFIXED64:
            return "bigint (int64)";
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.UINT64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FIXED64:
            return "bigint (uint64)";
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.BYTES:
            return "Uint8Array";
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.DOUBLE:
            return "number (float64)";
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FLOAT:
            return "number (float32)";
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FIXED32:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.UINT32:
            return "number (uint32)";
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.INT32:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SFIXED32:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SINT32:
            return "number (int32)";
    }
}


/***/ }),

/***/ 45640:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   T: () => (/* binding */ ConnectError)
/* harmony export */ });
/* harmony import */ var _code_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(16294);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(95444);
/* harmony import */ var _bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(57386);
/* harmony import */ var _protocol_connect_code_string_js__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(12386);
// Copyright 2021-2025 The Connect Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.



/**
 * ConnectError captures four pieces of information: a Code, an error
 * message, an optional cause of the error, and an optional collection of
 * arbitrary Protobuf messages called  "details".
 *
 * Because developer tools typically show just the error message, we prefix
 * it with the status code, so that the most important information is always
 * visible immediately.
 *
 * Error details are wrapped with google.protobuf.Any on the wire, so that
 * a server or middleware can attach arbitrary data to an error. Use the
 * method findDetails() to retrieve the details.
 */
class ConnectError extends Error {
    /**
     * Create a new ConnectError.
     * If no code is provided, code "unknown" is used.
     * Outgoing details are only relevant for the server side - a service may
     * raise an error with details, and it is up to the protocol implementation
     * to encode and send the details along with the error.
     */
    constructor(message, code = _code_js__WEBPACK_IMPORTED_MODULE_0__/* .Code */ .C.Unknown, metadata, outgoingDetails, cause) {
        super(createMessage(message, code));
        this.name = "ConnectError";
        // see https://www.typescriptlang.org/docs/handbook/release-notes/typescript-2-2.html#example
        Object.setPrototypeOf(this, new.target.prototype);
        this.rawMessage = message;
        this.code = code;
        this.metadata = new Headers(metadata !== null && metadata !== void 0 ? metadata : {});
        this.details = outgoingDetails !== null && outgoingDetails !== void 0 ? outgoingDetails : [];
        this.cause = cause;
    }
    /**
     * Convert any value - typically a caught error into a ConnectError,
     * following these rules:
     * - If the value is already a ConnectError, return it as is.
     * - If the value is an AbortError or TimeoutError from the fetch API, return
     *   the message of the error with code Canceled.
     * - For other Errors, return the error message with code Unknown by default.
     * - For other values, return the values String representation as a message,
     *   with the code Unknown by default.
     * The original value will be used for the "cause" property for the new
     * ConnectError.
     */
    static from(reason, code = _code_js__WEBPACK_IMPORTED_MODULE_0__/* .Code */ .C.Unknown) {
        if (reason instanceof ConnectError) {
            return reason;
        }
        if (reason instanceof Error) {
            if (reason.name == "AbortError" || reason.name == "TimeoutError") {
                // Fetch requests can only be canceled with an AbortController,
                // or with AbortSignal.timeout().
                return new ConnectError(reason.message, _code_js__WEBPACK_IMPORTED_MODULE_0__/* .Code */ .C.Canceled);
            }
            return new ConnectError(reason.message, code, undefined, undefined, reason);
        }
        return new ConnectError(String(reason), code, undefined, undefined, reason);
    }
    static [Symbol.hasInstance](v) {
        if (!(v instanceof Error)) {
            return false;
        }
        if (Object.getPrototypeOf(v) === ConnectError.prototype) {
            return true;
        }
        return (v.name === "ConnectError" &&
            "code" in v &&
            typeof v.code === "number" &&
            "metadata" in v &&
            "details" in v &&
            Array.isArray(v.details) &&
            "rawMessage" in v &&
            typeof v.rawMessage == "string" &&
            "cause" in v);
    }
    findDetails(typeOrRegistry) {
        const registry = typeOrRegistry.kind === "message"
            ? {
                getMessage: (typeName) => typeName === typeOrRegistry.typeName ? typeOrRegistry : undefined,
            }
            : typeOrRegistry;
        const details = [];
        for (const data of this.details) {
            if ("desc" in data) {
                if (registry.getMessage(data.desc.typeName)) {
                    details.push((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_1__/* .create */ .v)(data.desc, data.value));
                }
                continue;
            }
            const desc = registry.getMessage(data.type);
            if (desc) {
                try {
                    details.push((0,_bufbuild_protobuf__WEBPACK_IMPORTED_MODULE_2__/* .fromBinary */ .Er)(desc, data.value));
                }
                catch (_) {
                    // We silently give up if we are unable to parse the detail, because
                    // that appears to be the least worst behavior.
                    // It is very unlikely that a user surrounds a catch body handling the
                    // error with another try-catch statement, and we do not want to
                    // recommend doing so.
                }
            }
        }
        return details;
    }
}
/**
 * Create an error message, prefixing the given code.
 */
function createMessage(message, code) {
    return message.length
        ? `[${(0,_protocol_connect_code_string_js__WEBPACK_IMPORTED_MODULE_3__/* .codeToString */ .s)(code)}] ${message}`
        : `[${(0,_protocol_connect_code_string_js__WEBPACK_IMPORTED_MODULE_3__/* .codeToString */ .s)(code)}]`;
}


/***/ }),

/***/ 57386:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Er: () => (/* binding */ fromBinary),
/* harmony export */   dR: () => (/* binding */ readField)
/* harmony export */ });
/* unused harmony export mergeFromBinary */
/* harmony import */ var _descriptors_js__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(92806);
/* harmony import */ var _reflect_scalar_js__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(17134);
/* harmony import */ var _reflect_reflect_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(64683);
/* harmony import */ var _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(26811);
/* harmony import */ var _wire_varint_js__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(43848);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.





// Default options for parsing binary data.
const readDefaults = {
    readUnknownFields: true,
};
function makeReadOptions(options) {
    return options ? Object.assign(Object.assign({}, readDefaults), options) : readDefaults;
}
/**
 * Parse serialized binary data.
 */
function fromBinary(schema, bytes, options) {
    const msg = (0,_reflect_reflect_js__WEBPACK_IMPORTED_MODULE_0__/* .reflect */ .E4)(schema, undefined, false);
    readMessage(msg, new _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_1__/* .BinaryReader */ .V5(bytes), makeReadOptions(options), false, bytes.byteLength);
    return msg.message;
}
/**
 * Parse from binary data, merging fields.
 *
 * Repeated fields are appended. Map entries are added, overwriting
 * existing keys.
 *
 * If a message field is already present, it will be merged with the
 * new data.
 */
function mergeFromBinary(schema, target, bytes, options) {
    readMessage(reflect(schema, target, false), new BinaryReader(bytes), makeReadOptions(options), false, bytes.byteLength);
    return target;
}
/**
 * If `delimited` is false, read the length given in `lengthOrDelimitedFieldNo`.
 *
 * If `delimited` is true, read until an EndGroup tag. `lengthOrDelimitedFieldNo`
 * is the expected field number.
 *
 * @private
 */
function readMessage(message, reader, options, delimited, lengthOrDelimitedFieldNo) {
    var _a;
    const end = delimited ? reader.len : reader.pos + lengthOrDelimitedFieldNo;
    let fieldNo;
    let wireType;
    const unknownFields = (_a = message.getUnknown()) !== null && _a !== void 0 ? _a : [];
    while (reader.pos < end) {
        [fieldNo, wireType] = reader.tag();
        if (delimited && wireType == _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_1__/* .WireType */ .O0.EndGroup) {
            break;
        }
        const field = message.findNumber(fieldNo);
        if (!field) {
            const data = reader.skip(wireType, fieldNo);
            if (options.readUnknownFields) {
                unknownFields.push({ no: fieldNo, wireType, data });
            }
            continue;
        }
        readField(message, reader, field, wireType, options);
    }
    if (delimited) {
        if (wireType != _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_1__/* .WireType */ .O0.EndGroup || fieldNo !== lengthOrDelimitedFieldNo) {
            throw new Error("invalid end group tag");
        }
    }
    if (unknownFields.length > 0) {
        message.setUnknown(unknownFields);
    }
}
/**
 * @private
 */
function readField(message, reader, field, wireType, options) {
    var _a;
    switch (field.fieldKind) {
        case "scalar":
            message.set(field, readScalar(reader, field.scalar));
            break;
        case "enum":
            const val = readScalar(reader, _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.INT32);
            if (field.enum.open) {
                message.set(field, val);
            }
            else {
                const ok = field.enum.values.some((v) => v.number === val);
                if (ok) {
                    message.set(field, val);
                }
                else if (options.readUnknownFields) {
                    const bytes = [];
                    (0,_wire_varint_js__WEBPACK_IMPORTED_MODULE_3__/* .varint32write */ .f7)(val, bytes);
                    const unknownFields = (_a = message.getUnknown()) !== null && _a !== void 0 ? _a : [];
                    unknownFields.push({
                        no: field.number,
                        wireType,
                        data: new Uint8Array(bytes),
                    });
                    message.setUnknown(unknownFields);
                }
            }
            break;
        case "message":
            message.set(field, readMessageField(reader, options, field, message.get(field)));
            break;
        case "list":
            readListField(reader, wireType, message.get(field), options);
            break;
        case "map":
            readMapEntry(reader, message.get(field), options);
            break;
    }
}
// Read a map field, expecting key field = 1, value field = 2
function readMapEntry(reader, map, options) {
    const field = map.field();
    let key;
    let val;
    // Read the length of the map entry, which is a varint.
    const len = reader.uint32();
    // WARNING: Calculate end AFTER advancing reader.pos (above), so that
    //          reader.pos is at the start of the map entry.
    const end = reader.pos + len;
    while (reader.pos < end) {
        const [fieldNo] = reader.tag();
        switch (fieldNo) {
            case 1:
                key = readScalar(reader, field.mapKey);
                break;
            case 2:
                switch (field.mapKind) {
                    case "scalar":
                        val = readScalar(reader, field.scalar);
                        break;
                    case "enum":
                        val = reader.int32();
                        break;
                    case "message":
                        val = readMessageField(reader, options, field);
                        break;
                }
                break;
        }
    }
    if (key === undefined) {
        key = (0,_reflect_scalar_js__WEBPACK_IMPORTED_MODULE_4__/* .scalarZeroValue */ .M7)(field.mapKey, false);
    }
    if (val === undefined) {
        switch (field.mapKind) {
            case "scalar":
                val = (0,_reflect_scalar_js__WEBPACK_IMPORTED_MODULE_4__/* .scalarZeroValue */ .M7)(field.scalar, false);
                break;
            case "enum":
                val = field.enum.values[0].number;
                break;
            case "message":
                val = (0,_reflect_reflect_js__WEBPACK_IMPORTED_MODULE_0__/* .reflect */ .E4)(field.message, undefined, false);
                break;
        }
    }
    map.set(key, val);
}
function readListField(reader, wireType, list, options) {
    var _a;
    const field = list.field();
    if (field.listKind === "message") {
        list.add(readMessageField(reader, options, field));
        return;
    }
    const scalarType = (_a = field.scalar) !== null && _a !== void 0 ? _a : _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.INT32;
    const packed = wireType == _wire_binary_encoding_js__WEBPACK_IMPORTED_MODULE_1__/* .WireType */ .O0.LengthDelimited &&
        scalarType != _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.STRING &&
        scalarType != _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.BYTES;
    if (!packed) {
        list.add(readScalar(reader, scalarType));
        return;
    }
    const e = reader.uint32() + reader.pos;
    while (reader.pos < e) {
        list.add(readScalar(reader, scalarType));
    }
}
function readMessageField(reader, options, field, mergeMessage) {
    const delimited = field.delimitedEncoding;
    const message = mergeMessage !== null && mergeMessage !== void 0 ? mergeMessage : (0,_reflect_reflect_js__WEBPACK_IMPORTED_MODULE_0__/* .reflect */ .E4)(field.message, undefined, false);
    readMessage(message, reader, options, delimited, delimited ? field.number : reader.uint32());
    return message;
}
function readScalar(reader, type) {
    switch (type) {
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.STRING:
            return reader.string();
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.BOOL:
            return reader.bool();
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.DOUBLE:
            return reader.double();
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FLOAT:
            return reader.float();
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.INT32:
            return reader.int32();
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.INT64:
            return reader.int64();
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.UINT64:
            return reader.uint64();
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FIXED64:
            return reader.fixed64();
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.BYTES:
            return reader.bytes();
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.FIXED32:
            return reader.fixed32();
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SFIXED32:
            return reader.sfixed32();
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SFIXED64:
            return reader.sfixed64();
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SINT64:
            return reader.sint64();
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.UINT32:
            return reader.uint32();
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.SINT32:
            return reader.sint32();
    }
}


/***/ }),

/***/ 59233:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   d: () => (/* binding */ getTextEncoding)
/* harmony export */ });
/* unused harmony export configureTextEncoding */
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
const symbol = Symbol.for("@bufbuild/protobuf/text-encoding");
/**
 * Protobuf-ES requires the Text Encoding API to convert UTF-8 from and to
 * binary. This WHATWG API is widely available, but it is not part of the
 * ECMAScript standard. On runtimes where it is not available, use this
 * function to provide your own implementation.
 *
 * Note that the Text Encoding API does not provide a way to validate UTF-8.
 * Our implementation falls back to use encodeURIComponent().
 */
function configureTextEncoding(textEncoding) {
    globalThis[symbol] = textEncoding;
}
function getTextEncoding() {
    if (globalThis[symbol] == undefined) {
        const te = new globalThis.TextEncoder();
        const td = new globalThis.TextDecoder();
        globalThis[symbol] = {
            encodeUtf8(text) {
                return te.encode(text);
            },
            decodeUtf8(bytes) {
                return td.decode(bytes);
            },
            checkUtf8(text) {
                try {
                    encodeURIComponent(text);
                    return true;
                }
                catch (_) {
                    return false;
                }
            },
        };
    }
    return globalThis[symbol];
}


/***/ }),

/***/ 64444:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   b: () => (/* binding */ FieldError),
/* harmony export */   p: () => (/* binding */ isFieldError)
/* harmony export */ });
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
const errorNames = [
    "FieldValueInvalidError",
    "FieldListRangeError",
    "ForeignFieldError",
];
class FieldError extends Error {
    constructor(fieldOrOneof, message, name = "FieldValueInvalidError") {
        super(message);
        this.name = name;
        this.field = () => fieldOrOneof;
    }
}
function isFieldError(arg) {
    return (arg instanceof Error &&
        errorNames.includes(arg.name) &&
        "field" in arg &&
        typeof arg.field == "function");
}


/***/ }),

/***/ 64683:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   E4: () => (/* binding */ reflect)
/* harmony export */ });
/* unused harmony exports reflectList, reflectMap */
/* harmony import */ var _descriptors_js__WEBPACK_IMPORTED_MODULE_7__ = __webpack_require__(92806);
/* harmony import */ var _reflect_check_js__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(45394);
/* harmony import */ var _error_js__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(64444);
/* harmony import */ var _unsafe_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(22060);
/* harmony import */ var _create_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(95444);
/* harmony import */ var _wkt_wrappers_js__WEBPACK_IMPORTED_MODULE_6__ = __webpack_require__(34123);
/* harmony import */ var _scalar_js__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(17134);
/* harmony import */ var _proto_int64_js__WEBPACK_IMPORTED_MODULE_8__ = __webpack_require__(2952);
/* harmony import */ var _guard_js__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(71035);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.









/**
 * Create a ReflectMessage.
 */
function reflect(messageDesc, message, 
/**
 * By default, field values are validated when setting them. For example,
 * a value for an uint32 field must be a ECMAScript Number >= 0.
 *
 * When field values are trusted, performance can be improved by disabling
 * checks.
 */
check = true) {
    return new ReflectMessageImpl(messageDesc, message, check);
}
const messageSortedFields = new WeakMap();
class ReflectMessageImpl {
    get sortedFields() {
        const cached = messageSortedFields.get(this.desc);
        if (cached) {
            return cached;
        }
        const sortedFields = this.desc.fields
            .concat()
            .sort((a, b) => a.number - b.number);
        messageSortedFields.set(this.desc, sortedFields);
        return sortedFields;
    }
    constructor(messageDesc, message, check = true) {
        this.lists = new Map();
        this.maps = new Map();
        this.check = check;
        this.desc = messageDesc;
        this.message = this[_unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeLocal */ .I4] = message !== null && message !== void 0 ? message : (0,_create_js__WEBPACK_IMPORTED_MODULE_1__/* .create */ .v)(messageDesc);
        this.fields = messageDesc.fields;
        this.oneofs = messageDesc.oneofs;
        this.members = messageDesc.members;
    }
    findNumber(number) {
        if (!this._fieldsByNumber) {
            this._fieldsByNumber = new Map(this.desc.fields.map((f) => [f.number, f]));
        }
        return this._fieldsByNumber.get(number);
    }
    oneofCase(oneof) {
        assertOwn(this.message, oneof);
        return (0,_unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeOneofCase */ .mO)(this.message, oneof);
    }
    isSet(field) {
        assertOwn(this.message, field);
        return (0,_unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeIsSet */ .zB)(this.message, field);
    }
    clear(field) {
        assertOwn(this.message, field);
        (0,_unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeClear */ .yO)(this.message, field);
    }
    get(field) {
        assertOwn(this.message, field);
        const value = (0,_unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeGet */ .$v)(this.message, field);
        switch (field.fieldKind) {
            case "list":
                // eslint-disable-next-line no-case-declarations
                let list = this.lists.get(field);
                if (!list || list[_unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeLocal */ .I4] !== value) {
                    this.lists.set(field, 
                    // biome-ignore lint/suspicious/noAssignInExpressions: no
                    (list = new ReflectListImpl(field, value, this.check)));
                }
                return list;
            case "map":
                let map = this.maps.get(field);
                if (!map || map[_unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeLocal */ .I4] !== value) {
                    this.maps.set(field, 
                    // biome-ignore lint/suspicious/noAssignInExpressions: no
                    (map = new ReflectMapImpl(field, value, this.check)));
                }
                return map;
            case "message":
                return messageToReflect(field, value, this.check);
            case "scalar":
                return (value === undefined
                    ? (0,_scalar_js__WEBPACK_IMPORTED_MODULE_2__/* .scalarZeroValue */ .M7)(field.scalar, false)
                    : longToReflect(field, value));
            case "enum":
                return (value !== null && value !== void 0 ? value : field.enum.values[0].number);
        }
    }
    set(field, value) {
        assertOwn(this.message, field);
        if (this.check) {
            const err = (0,_reflect_check_js__WEBPACK_IMPORTED_MODULE_3__/* .checkField */ .PZ)(field, value);
            if (err) {
                throw err;
            }
        }
        let local;
        if (field.fieldKind == "message") {
            local = messageToLocal(field, value);
        }
        else if ((0,_guard_js__WEBPACK_IMPORTED_MODULE_4__/* .isReflectMap */ .G6)(value) || (0,_guard_js__WEBPACK_IMPORTED_MODULE_4__/* .isReflectList */ .sG)(value)) {
            local = value[_unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeLocal */ .I4];
        }
        else {
            local = longToLocal(field, value);
        }
        (0,_unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeSet */ .lA)(this.message, field, local);
    }
    getUnknown() {
        return this.message.$unknown;
    }
    setUnknown(value) {
        this.message.$unknown = value;
    }
}
function assertOwn(owner, member) {
    if (member.parent.typeName !== owner.$typeName) {
        throw new _error_js__WEBPACK_IMPORTED_MODULE_5__/* .FieldError */ .b(member, `cannot use ${member.toString()} with message ${owner.$typeName}`, "ForeignFieldError");
    }
}
/**
 * Create a ReflectList.
 */
function reflectList(field, unsafeInput, 
/**
 * By default, field values are validated when setting them. For example,
 * a value for an uint32 field must be a ECMAScript Number >= 0.
 *
 * When field values are trusted, performance can be improved by disabling
 * checks.
 */
check = true) {
    return new ReflectListImpl(field, unsafeInput !== null && unsafeInput !== void 0 ? unsafeInput : [], check);
}
class ReflectListImpl {
    field() {
        return this._field;
    }
    get size() {
        return this._arr.length;
    }
    constructor(field, unsafeInput, check) {
        this._field = field;
        this._arr = this[_unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeLocal */ .I4] = unsafeInput;
        this.check = check;
    }
    get(index) {
        const item = this._arr[index];
        return item === undefined
            ? undefined
            : listItemToReflect(this._field, item, this.check);
    }
    set(index, item) {
        if (index < 0 || index >= this._arr.length) {
            throw new _error_js__WEBPACK_IMPORTED_MODULE_5__/* .FieldError */ .b(this._field, `list item #${index + 1}: out of range`);
        }
        if (this.check) {
            const err = (0,_reflect_check_js__WEBPACK_IMPORTED_MODULE_3__/* .checkListItem */ .MJ)(this._field, index, item);
            if (err) {
                throw err;
            }
        }
        this._arr[index] = listItemToLocal(this._field, item);
    }
    add(item) {
        if (this.check) {
            const err = (0,_reflect_check_js__WEBPACK_IMPORTED_MODULE_3__/* .checkListItem */ .MJ)(this._field, this._arr.length, item);
            if (err) {
                throw err;
            }
        }
        this._arr.push(listItemToLocal(this._field, item));
        return undefined;
    }
    clear() {
        this._arr.splice(0, this._arr.length);
    }
    [Symbol.iterator]() {
        return this.values();
    }
    keys() {
        return this._arr.keys();
    }
    *values() {
        for (const item of this._arr) {
            yield listItemToReflect(this._field, item, this.check);
        }
    }
    *entries() {
        for (let i = 0; i < this._arr.length; i++) {
            yield [i, listItemToReflect(this._field, this._arr[i], this.check)];
        }
    }
}
/**
 * Create a ReflectMap.
 */
function reflectMap(field, unsafeInput, 
/**
 * By default, field values are validated when setting them. For example,
 * a value for an uint32 field must be a ECMAScript Number >= 0.
 *
 * When field values are trusted, performance can be improved by disabling
 * checks.
 */
check = true) {
    return new ReflectMapImpl(field, unsafeInput, check);
}
class ReflectMapImpl {
    constructor(field, unsafeInput, check = true) {
        this.obj = this[_unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeLocal */ .I4] = unsafeInput !== null && unsafeInput !== void 0 ? unsafeInput : {};
        this.check = check;
        this._field = field;
    }
    field() {
        return this._field;
    }
    set(key, value) {
        if (this.check) {
            const err = (0,_reflect_check_js__WEBPACK_IMPORTED_MODULE_3__/* .checkMapEntry */ .xI)(this._field, key, value);
            if (err) {
                throw err;
            }
        }
        this.obj[mapKeyToLocal(key)] = mapValueToLocal(this._field, value);
        return this;
    }
    delete(key) {
        const k = mapKeyToLocal(key);
        const has = Object.prototype.hasOwnProperty.call(this.obj, k);
        if (has) {
            delete this.obj[k];
        }
        return has;
    }
    clear() {
        for (const key of Object.keys(this.obj)) {
            delete this.obj[key];
        }
    }
    get(key) {
        let val = this.obj[mapKeyToLocal(key)];
        if (val !== undefined) {
            val = mapValueToReflect(this._field, val, this.check);
        }
        return val;
    }
    has(key) {
        return Object.prototype.hasOwnProperty.call(this.obj, mapKeyToLocal(key));
    }
    *keys() {
        for (const objKey of Object.keys(this.obj)) {
            yield mapKeyToReflect(objKey, this._field.mapKey);
        }
    }
    *entries() {
        for (const objEntry of Object.entries(this.obj)) {
            yield [
                mapKeyToReflect(objEntry[0], this._field.mapKey),
                mapValueToReflect(this._field, objEntry[1], this.check),
            ];
        }
    }
    [Symbol.iterator]() {
        return this.entries();
    }
    get size() {
        return Object.keys(this.obj).length;
    }
    *values() {
        for (const val of Object.values(this.obj)) {
            yield mapValueToReflect(this._field, val, this.check);
        }
    }
    forEach(callbackfn, thisArg) {
        for (const mapEntry of this.entries()) {
            callbackfn.call(thisArg, mapEntry[1], mapEntry[0], this);
        }
    }
}
function messageToLocal(field, value) {
    if (!(0,_guard_js__WEBPACK_IMPORTED_MODULE_4__/* .isReflectMessage */ .LT)(value)) {
        return value;
    }
    if ((0,_wkt_wrappers_js__WEBPACK_IMPORTED_MODULE_6__/* .isWrapper */ .Y)(value.message) &&
        !field.oneof &&
        field.fieldKind == "message") {
        // Types from google/protobuf/wrappers.proto are unwrapped when used in
        // a singular field that is not part of a oneof group.
        return value.message.value;
    }
    if (value.desc.typeName == "google.protobuf.Struct" &&
        field.parent.typeName != "google.protobuf.Value") {
        // google.protobuf.Struct is represented with JsonObject when used in a
        // field, except when used in google.protobuf.Value.
        return wktStructToLocal(value.message);
    }
    return value.message;
}
function messageToReflect(field, value, check) {
    if (value !== undefined) {
        if ((0,_wkt_wrappers_js__WEBPACK_IMPORTED_MODULE_6__/* .isWrapperDesc */ .l)(field.message) &&
            !field.oneof &&
            field.fieldKind == "message") {
            // Types from google/protobuf/wrappers.proto are unwrapped when used in
            // a singular field that is not part of a oneof group.
            value = {
                $typeName: field.message.typeName,
                value: longToReflect(field.message.fields[0], value),
            };
        }
        else if (field.message.typeName == "google.protobuf.Struct" &&
            field.parent.typeName != "google.protobuf.Value" &&
            (0,_guard_js__WEBPACK_IMPORTED_MODULE_4__/* .isObject */ .Gv)(value)) {
            // google.protobuf.Struct is represented with JsonObject when used in a
            // field, except when used in google.protobuf.Value.
            value = wktStructToReflect(value);
        }
    }
    return new ReflectMessageImpl(field.message, value, check);
}
function listItemToLocal(field, value) {
    if (field.listKind == "message") {
        return messageToLocal(field, value);
    }
    return longToLocal(field, value);
}
function listItemToReflect(field, value, check) {
    if (field.listKind == "message") {
        return messageToReflect(field, value, check);
    }
    return longToReflect(field, value);
}
function mapValueToLocal(field, value) {
    if (field.mapKind == "message") {
        return messageToLocal(field, value);
    }
    return longToLocal(field, value);
}
function mapValueToReflect(field, value, check) {
    if (field.mapKind == "message") {
        return messageToReflect(field, value, check);
    }
    return value;
}
function mapKeyToLocal(key) {
    return typeof key == "string" || typeof key == "number" ? key : String(key);
}
/**
 * Converts a map key (any scalar value except float, double, or bytes) from its
 * representation in a message (string or number, the only possible object key
 * types) to the closest possible type in ECMAScript.
 */
function mapKeyToReflect(key, type) {
    switch (type) {
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.STRING:
            return key;
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.INT32:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.FIXED32:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.UINT32:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.SFIXED32:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.SINT32: {
            const n = Number.parseInt(key);
            if (Number.isFinite(n)) {
                return n;
            }
            break;
        }
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.BOOL:
            switch (key) {
                case "true":
                    return true;
                case "false":
                    return false;
            }
            break;
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.UINT64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.FIXED64:
            try {
                return _proto_int64_js__WEBPACK_IMPORTED_MODULE_8__/* .protoInt64 */ .M.uParse(key);
            }
            catch (_a) {
                //
            }
            break;
        default:
            // INT64, SFIXED64, SINT64
            try {
                return _proto_int64_js__WEBPACK_IMPORTED_MODULE_8__/* .protoInt64 */ .M.parse(key);
            }
            catch (_b) {
                //
            }
            break;
    }
    return key;
}
function longToReflect(field, value) {
    switch (field.scalar) {
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.INT64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.SFIXED64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.SINT64:
            if ("longAsString" in field &&
                field.longAsString &&
                typeof value == "string") {
                value = _proto_int64_js__WEBPACK_IMPORTED_MODULE_8__/* .protoInt64 */ .M.parse(value);
            }
            break;
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.FIXED64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.UINT64:
            if ("longAsString" in field &&
                field.longAsString &&
                typeof value == "string") {
                value = _proto_int64_js__WEBPACK_IMPORTED_MODULE_8__/* .protoInt64 */ .M.uParse(value);
            }
            break;
    }
    return value;
}
function longToLocal(field, value) {
    switch (field.scalar) {
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.INT64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.SFIXED64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.SINT64:
            if ("longAsString" in field && field.longAsString) {
                value = String(value);
            }
            else if (typeof value == "string" || typeof value == "number") {
                value = _proto_int64_js__WEBPACK_IMPORTED_MODULE_8__/* .protoInt64 */ .M.parse(value);
            }
            break;
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.FIXED64:
        case _descriptors_js__WEBPACK_IMPORTED_MODULE_7__/* .ScalarType */ .L.UINT64:
            if ("longAsString" in field && field.longAsString) {
                value = String(value);
            }
            else if (typeof value == "string" || typeof value == "number") {
                value = _proto_int64_js__WEBPACK_IMPORTED_MODULE_8__/* .protoInt64 */ .M.uParse(value);
            }
            break;
    }
    return value;
}
function wktStructToReflect(json) {
    const struct = {
        $typeName: "google.protobuf.Struct",
        fields: {},
    };
    if ((0,_guard_js__WEBPACK_IMPORTED_MODULE_4__/* .isObject */ .Gv)(json)) {
        for (const [k, v] of Object.entries(json)) {
            struct.fields[k] = wktValueToReflect(v);
        }
    }
    return struct;
}
function wktStructToLocal(val) {
    const json = {};
    for (const [k, v] of Object.entries(val.fields)) {
        json[k] = wktValueToLocal(v);
    }
    return json;
}
function wktValueToLocal(val) {
    switch (val.kind.case) {
        case "structValue":
            return wktStructToLocal(val.kind.value);
        case "listValue":
            return val.kind.value.values.map(wktValueToLocal);
        case "nullValue":
        case undefined:
            return null;
        default:
            return val.kind.value;
    }
}
function wktValueToReflect(json) {
    const value = {
        $typeName: "google.protobuf.Value",
        kind: { case: undefined },
    };
    switch (typeof json) {
        case "number":
            value.kind = { case: "numberValue", value: json };
            break;
        case "string":
            value.kind = { case: "stringValue", value: json };
            break;
        case "boolean":
            value.kind = { case: "boolValue", value: json };
            break;
        case "object":
            if (json === null) {
                const nullValue = 0;
                value.kind = { case: "nullValue", value: nullValue };
            }
            else if (Array.isArray(json)) {
                const listValue = {
                    $typeName: "google.protobuf.ListValue",
                    values: [],
                };
                if (Array.isArray(json)) {
                    for (const e of json) {
                        listValue.values.push(wktValueToReflect(e));
                    }
                }
                value.kind = {
                    case: "listValue",
                    value: listValue,
                };
            }
            else {
                value.kind = {
                    case: "structValue",
                    value: wktStructToReflect(json),
                };
            }
            break;
    }
    return value;
}


/***/ }),

/***/ 71035:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   G6: () => (/* binding */ isReflectMap),
/* harmony export */   Gv: () => (/* binding */ isObject),
/* harmony export */   LT: () => (/* binding */ isReflectMessage),
/* harmony export */   sG: () => (/* binding */ isReflectList)
/* harmony export */ });
/* unused harmony export isOneofADT */
/* harmony import */ var _unsafe_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(22060);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

function isObject(arg) {
    return arg !== null && typeof arg == "object" && !Array.isArray(arg);
}
function isOneofADT(arg) {
    return (arg !== null &&
        typeof arg == "object" &&
        "case" in arg &&
        ((typeof arg.case == "string" && "value" in arg && arg.value != null) ||
            (arg.case === undefined &&
                (!("value" in arg) || arg.value === undefined))));
}
function isReflectList(arg, field) {
    var _a, _b, _c, _d;
    if (isObject(arg) &&
        _unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeLocal */ .I4 in arg &&
        "add" in arg &&
        "field" in arg &&
        typeof arg.field == "function") {
        if (field !== undefined) {
            const a = field;
            const b = arg.field();
            return (a.listKind == b.listKind &&
                a.scalar === b.scalar &&
                ((_a = a.message) === null || _a === void 0 ? void 0 : _a.typeName) === ((_b = b.message) === null || _b === void 0 ? void 0 : _b.typeName) &&
                ((_c = a.enum) === null || _c === void 0 ? void 0 : _c.typeName) === ((_d = b.enum) === null || _d === void 0 ? void 0 : _d.typeName));
        }
        return true;
    }
    return false;
}
function isReflectMap(arg, field) {
    var _a, _b, _c, _d;
    if (isObject(arg) &&
        _unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeLocal */ .I4 in arg &&
        "has" in arg &&
        "field" in arg &&
        typeof arg.field == "function") {
        if (field !== undefined) {
            const a = field, b = arg.field();
            return (a.mapKey === b.mapKey &&
                a.mapKind == b.mapKind &&
                a.scalar === b.scalar &&
                ((_a = a.message) === null || _a === void 0 ? void 0 : _a.typeName) === ((_b = b.message) === null || _b === void 0 ? void 0 : _b.typeName) &&
                ((_c = a.enum) === null || _c === void 0 ? void 0 : _c.typeName) === ((_d = b.enum) === null || _d === void 0 ? void 0 : _d.typeName));
        }
        return true;
    }
    return false;
}
function isReflectMessage(arg, messageDesc) {
    return (isObject(arg) &&
        _unsafe_js__WEBPACK_IMPORTED_MODULE_0__/* .unsafeLocal */ .I4 in arg &&
        "desc" in arg &&
        isObject(arg.desc) &&
        arg.desc.kind === "message" &&
        (messageDesc === undefined || arg.desc.typeName == messageDesc.typeName));
}


/***/ }),

/***/ 92806:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   L: () => (/* binding */ ScalarType)
/* harmony export */ });
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
/**
 * Scalar value types. This is a subset of field types declared by protobuf
 * enum google.protobuf.FieldDescriptorProto.Type The types GROUP and MESSAGE
 * are omitted, but the numerical values are identical.
 */
var ScalarType;
(function (ScalarType) {
    // 0 is reserved for errors.
    // Order is weird for historical reasons.
    ScalarType[ScalarType["DOUBLE"] = 1] = "DOUBLE";
    ScalarType[ScalarType["FLOAT"] = 2] = "FLOAT";
    // Not ZigZag encoded.  Negative numbers take 10 bytes.  Use TYPE_SINT64 if
    // negative values are likely.
    ScalarType[ScalarType["INT64"] = 3] = "INT64";
    ScalarType[ScalarType["UINT64"] = 4] = "UINT64";
    // Not ZigZag encoded.  Negative numbers take 10 bytes.  Use TYPE_SINT32 if
    // negative values are likely.
    ScalarType[ScalarType["INT32"] = 5] = "INT32";
    ScalarType[ScalarType["FIXED64"] = 6] = "FIXED64";
    ScalarType[ScalarType["FIXED32"] = 7] = "FIXED32";
    ScalarType[ScalarType["BOOL"] = 8] = "BOOL";
    ScalarType[ScalarType["STRING"] = 9] = "STRING";
    // Tag-delimited aggregate.
    // Group type is deprecated and not supported in proto3. However, Proto3
    // implementations should still be able to parse the group wire format and
    // treat group fields as unknown fields.
    // TYPE_GROUP = 10,
    // TYPE_MESSAGE = 11,  // Length-delimited aggregate.
    // New in version 2.
    ScalarType[ScalarType["BYTES"] = 12] = "BYTES";
    ScalarType[ScalarType["UINT32"] = 13] = "UINT32";
    // TYPE_ENUM = 14,
    ScalarType[ScalarType["SFIXED32"] = 15] = "SFIXED32";
    ScalarType[ScalarType["SFIXED64"] = 16] = "SFIXED64";
    ScalarType[ScalarType["SINT32"] = 17] = "SINT32";
    ScalarType[ScalarType["SINT64"] = 18] = "SINT64";
})(ScalarType || (ScalarType = {}));


/***/ }),

/***/ 95444:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   v: () => (/* binding */ create)
/* harmony export */ });
/* harmony import */ var _is_message_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(36170);
/* harmony import */ var _descriptors_js__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(92806);
/* harmony import */ var _reflect_scalar_js__WEBPACK_IMPORTED_MODULE_5__ = __webpack_require__(17134);
/* harmony import */ var _reflect_guard_js__WEBPACK_IMPORTED_MODULE_3__ = __webpack_require__(71035);
/* harmony import */ var _reflect_unsafe_js__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(22060);
/* harmony import */ var _wkt_wrappers_js__WEBPACK_IMPORTED_MODULE_4__ = __webpack_require__(34123);
// Copyright 2021-2026 Buf Technologies, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.






// bootstrap-inject google.protobuf.Edition.EDITION_PROTO3: const $name: Edition.$localName = $number;
const EDITION_PROTO3 = 999;
// bootstrap-inject google.protobuf.Edition.EDITION_PROTO2: const $name: Edition.$localName = $number;
const EDITION_PROTO2 = 998;
// bootstrap-inject google.protobuf.FeatureSet.FieldPresence.IMPLICIT: const $name: FeatureSet_FieldPresence.$localName = $number;
const IMPLICIT = 2;
/**
 * Create a new message instance.
 *
 * The second argument is an optional initializer object, where all fields are
 * optional.
 */
function create(schema, init) {
    if ((0,_is_message_js__WEBPACK_IMPORTED_MODULE_0__/* .isMessage */ .K)(init, schema)) {
        return init;
    }
    const message = createZeroMessage(schema);
    if (init !== undefined) {
        initMessage(schema, message, init);
    }
    return message;
}
/**
 * Sets field values from a MessageInitShape on a zero message.
 */
function initMessage(messageDesc, message, init) {
    for (const member of messageDesc.members) {
        let value = init[member.localName];
        if (value == null) {
            // intentionally ignore undefined and null
            continue;
        }
        let field;
        if (member.kind == "oneof") {
            const oneofField = (0,_reflect_unsafe_js__WEBPACK_IMPORTED_MODULE_1__/* .unsafeOneofCase */ .mO)(init, member);
            if (!oneofField) {
                continue;
            }
            field = oneofField;
            value = (0,_reflect_unsafe_js__WEBPACK_IMPORTED_MODULE_1__/* .unsafeGet */ .$v)(init, oneofField);
        }
        else {
            field = member;
        }
        switch (field.fieldKind) {
            case "message":
                value = toMessage(field, value);
                break;
            case "scalar":
                value = initScalar(field, value);
                break;
            case "list":
                value = initList(field, value);
                break;
            case "map":
                value = initMap(field, value);
                break;
        }
        (0,_reflect_unsafe_js__WEBPACK_IMPORTED_MODULE_1__/* .unsafeSet */ .lA)(message, field, value);
    }
    return message;
}
function initScalar(field, value) {
    if (field.scalar == _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.BYTES) {
        return toU8Arr(value);
    }
    return value;
}
function initMap(field, value) {
    if ((0,_reflect_guard_js__WEBPACK_IMPORTED_MODULE_3__/* .isObject */ .Gv)(value)) {
        if (field.scalar == _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.BYTES) {
            return convertObjectValues(value, toU8Arr);
        }
        if (field.mapKind == "message") {
            return convertObjectValues(value, (val) => toMessage(field, val));
        }
    }
    return value;
}
function initList(field, value) {
    if (Array.isArray(value)) {
        if (field.scalar == _descriptors_js__WEBPACK_IMPORTED_MODULE_2__/* .ScalarType */ .L.BYTES) {
            return value.map(toU8Arr);
        }
        if (field.listKind == "message") {
            return value.map((item) => toMessage(field, item));
        }
    }
    return value;
}
function toMessage(field, value) {
    if (field.fieldKind == "message" &&
        !field.oneof &&
        (0,_wkt_wrappers_js__WEBPACK_IMPORTED_MODULE_4__/* .isWrapperDesc */ .l)(field.message)) {
        // Types from google/protobuf/wrappers.proto are unwrapped when used in
        // a singular field that is not part of a oneof group.
        return initScalar(field.message.fields[0], value);
    }
    if ((0,_reflect_guard_js__WEBPACK_IMPORTED_MODULE_3__/* .isObject */ .Gv)(value)) {
        if (field.message.typeName == "google.protobuf.Struct" &&
            field.parent.typeName !== "google.protobuf.Value") {
            // google.protobuf.Struct is represented with JsonObject when used in a
            // field, except when used in google.protobuf.Value.
            return value;
        }
        if (!(0,_is_message_js__WEBPACK_IMPORTED_MODULE_0__/* .isMessage */ .K)(value, field.message)) {
            return create(field.message, value);
        }
    }
    return value;
}
// converts any ArrayLike<number> to Uint8Array if necessary.
function toU8Arr(value) {
    return Array.isArray(value) ? new Uint8Array(value) : value;
}
function convertObjectValues(obj, fn) {
    const ret = {};
    for (const entry of Object.entries(obj)) {
        ret[entry[0]] = fn(entry[1]);
    }
    return ret;
}
const tokenZeroMessageField = Symbol();
const messagePrototypes = new WeakMap();
/**
 * Create a zero message.
 */
function createZeroMessage(desc) {
    let msg;
    if (!needsPrototypeChain(desc)) {
        msg = {
            $typeName: desc.typeName,
        };
        for (const member of desc.members) {
            if (member.kind == "oneof" || member.presence == IMPLICIT) {
                msg[member.localName] = createZeroField(member);
            }
        }
    }
    else {
        // Support default values and track presence via the prototype chain
        const cached = messagePrototypes.get(desc);
        let prototype;
        let members;
        if (cached) {
            ({ prototype, members } = cached);
        }
        else {
            prototype = {};
            members = new Set();
            for (const member of desc.members) {
                if (member.kind == "oneof") {
                    // we can only put immutable values on the prototype,
                    // oneof ADTs are mutable
                    continue;
                }
                if (member.fieldKind != "scalar" && member.fieldKind != "enum") {
                    // only scalar and enum values are immutable, map, list, and message
                    // are not
                    continue;
                }
                if (member.presence == IMPLICIT) {
                    // implicit presence tracks field presence by zero values - e.g. 0, false, "", are unset, 1, true, "x" are set.
                    // message, map, list fields are mutable, and also have IMPLICIT presence.
                    continue;
                }
                members.add(member);
                prototype[member.localName] = createZeroField(member);
            }
            messagePrototypes.set(desc, { prototype, members });
        }
        msg = Object.create(prototype);
        msg.$typeName = desc.typeName;
        for (const member of desc.members) {
            if (members.has(member)) {
                continue;
            }
            if (member.kind == "field") {
                if (member.fieldKind == "message") {
                    continue;
                }
                if (member.fieldKind == "scalar" || member.fieldKind == "enum") {
                    if (member.presence != IMPLICIT) {
                        continue;
                    }
                }
            }
            msg[member.localName] = createZeroField(member);
        }
    }
    return msg;
}
/**
 * Do we need the prototype chain to track field presence?
 */
function needsPrototypeChain(desc) {
    switch (desc.file.edition) {
        case EDITION_PROTO3:
            // proto3 always uses implicit presence, we never need the prototype chain.
            return false;
        case EDITION_PROTO2:
            // proto2 never uses implicit presence, we always need the prototype chain.
            return true;
        default:
            // If a message uses scalar or enum fields with explicit presence, we need
            // the prototype chain to track presence. This rule does not apply to fields
            // in a oneof group - they use a different mechanism to track presence.
            return desc.fields.some((f) => f.presence != IMPLICIT && f.fieldKind != "message" && !f.oneof);
    }
}
/**
 * Returns a zero value for oneof groups, and for every field kind except
 * messages. Scalar and enum fields can have default values.
 */
function createZeroField(field) {
    if (field.kind == "oneof") {
        return { case: undefined };
    }
    if (field.fieldKind == "list") {
        return [];
    }
    if (field.fieldKind == "map") {
        return {}; // Object.create(null) would be desirable here, but is unsupported by react https://react.dev/reference/react/use-server#serializable-parameters-and-return-values
    }
    if (field.fieldKind == "message") {
        return tokenZeroMessageField;
    }
    const defaultValue = field.getDefaultValue();
    if (defaultValue !== undefined) {
        return field.fieldKind == "scalar" && field.longAsString
            ? defaultValue.toString()
            : defaultValue;
    }
    return field.fieldKind == "scalar"
        ? (0,_reflect_scalar_js__WEBPACK_IMPORTED_MODULE_5__/* .scalarZeroValue */ .M7)(field.scalar, field.longAsString)
        : field.enum.values[0].number;
}


/***/ })

}]);
//# sourceMappingURL=4202-1e943ef6ea9cb7a6.js.map