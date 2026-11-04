(self["webpackChunk_N_E"] = self["webpackChunk_N_E"] || []).push([[5857],{

/***/ 14376:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 17830:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 24100:
/***/ ((__unused_webpack_module, exports) => {

"use strict";

Object.defineProperty(exports, "__esModule", ({ value: true }));
class EventQueue {
    constructor() {
        this.pullQueue = [];
        this.pushQueue = [];
        this.eventHandlers = {};
        this.isPaused = false;
        this.isStopped = false;
    }
    push(value) {
        if (this.isStopped)
            return;
        const resolution = { value, done: false };
        if (this.pullQueue.length) {
            const placeholder = this.pullQueue.shift();
            if (placeholder)
                placeholder.resolve(resolution);
        }
        else {
            this.pushQueue.push(Promise.resolve(resolution));
            if (this.highWaterMark !== undefined &&
                this.pushQueue.length >= this.highWaterMark &&
                !this.isPaused) {
                this.isPaused = true;
                if (this.eventHandlers.highWater) {
                    this.eventHandlers.highWater();
                }
                else if (console) {
                    console.warn(`EventIterator queue reached ${this.pushQueue.length} items`);
                }
            }
        }
    }
    stop() {
        if (this.isStopped)
            return;
        this.isStopped = true;
        this.remove();
        for (const placeholder of this.pullQueue) {
            placeholder.resolve({ value: undefined, done: true });
        }
        this.pullQueue.length = 0;
    }
    fail(error) {
        if (this.isStopped)
            return;
        this.isStopped = true;
        this.remove();
        if (this.pullQueue.length) {
            for (const placeholder of this.pullQueue) {
                placeholder.reject(error);
            }
            this.pullQueue.length = 0;
        }
        else {
            const rejection = Promise.reject(error);
            /* Attach error handler to avoid leaking an unhandled promise rejection. */
            rejection.catch(() => { });
            this.pushQueue.push(rejection);
        }
    }
    remove() {
        Promise.resolve().then(() => {
            if (this.removeCallback)
                this.removeCallback();
        });
    }
    [Symbol.asyncIterator]() {
        return {
            next: (value) => {
                const result = this.pushQueue.shift();
                if (result) {
                    if (this.lowWaterMark !== undefined &&
                        this.pushQueue.length <= this.lowWaterMark &&
                        this.isPaused) {
                        this.isPaused = false;
                        if (this.eventHandlers.lowWater) {
                            this.eventHandlers.lowWater();
                        }
                    }
                    return result;
                }
                else if (this.isStopped) {
                    return Promise.resolve({ value: undefined, done: true });
                }
                else {
                    return new Promise((resolve, reject) => {
                        this.pullQueue.push({ resolve, reject });
                    });
                }
            },
            return: () => {
                this.isStopped = true;
                this.pushQueue.length = 0;
                this.remove();
                return Promise.resolve({ value: undefined, done: true });
            },
        };
    }
}
class EventIterator {
    constructor(listen, { highWaterMark = 100, lowWaterMark = 1 } = {}) {
        const queue = new EventQueue();
        queue.highWaterMark = highWaterMark;
        queue.lowWaterMark = lowWaterMark;
        queue.removeCallback =
            listen({
                push: value => queue.push(value),
                stop: () => queue.stop(),
                fail: error => queue.fail(error),
                on: (event, fn) => {
                    queue.eventHandlers[event] = fn;
                },
            }) || (() => { });
        this[Symbol.asyncIterator] = () => queue[Symbol.asyncIterator]();
        Object.freeze(this);
    }
}
exports.EventIterator = EventIterator;
exports["default"] = EventIterator;


/***/ }),

/***/ 34897:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 45238:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 46849:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 58471:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 80370:
/***/ (() => {

// extracted by mini-css-extract-plugin

/***/ }),

/***/ 87679:
/***/ ((__unused_webpack_module, exports, __webpack_require__) => {

"use strict";
var __webpack_unused_export__;

__webpack_unused_export__ = ({ value: true });
const event_iterator_1 = __webpack_require__(24100);
exports.PP = event_iterator_1.EventIterator;
function subscribe(event, options, evOptions) {
    return new event_iterator_1.EventIterator(({ push }) => {
        this.addEventListener(event, push, options);
        return () => this.removeEventListener(event, push, options);
    }, evOptions);
}
__webpack_unused_export__ = subscribe;
__webpack_unused_export__ = event_iterator_1.EventIterator;


/***/ }),

/***/ 92609:
/***/ ((__unused_webpack___webpack_module__, __webpack_exports__, __webpack_require__) => {

"use strict";

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  N: () => (/* binding */ connect)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base10.js
var base10_namespaceObject = {};
__webpack_require__.r(base10_namespaceObject);
__webpack_require__.d(base10_namespaceObject, {
  base10: () => (base10)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base16.js
var base16_namespaceObject = {};
__webpack_require__.r(base16_namespaceObject);
__webpack_require__.d(base16_namespaceObject, {
  base16: () => (base16),
  base16upper: () => (base16upper)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base2.js
var base2_namespaceObject = {};
__webpack_require__.r(base2_namespaceObject);
__webpack_require__.d(base2_namespaceObject, {
  base2: () => (base2)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base256emoji.js
var base256emoji_namespaceObject = {};
__webpack_require__.r(base256emoji_namespaceObject);
__webpack_require__.d(base256emoji_namespaceObject, {
  base256emoji: () => (base256emoji)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base32.js
var base32_namespaceObject = {};
__webpack_require__.r(base32_namespaceObject);
__webpack_require__.d(base32_namespaceObject, {
  base32: () => (base32),
  base32hex: () => (base32hex),
  base32hexpad: () => (base32hexpad),
  base32hexpadupper: () => (base32hexpadupper),
  base32hexupper: () => (base32hexupper),
  base32pad: () => (base32pad),
  base32padupper: () => (base32padupper),
  base32upper: () => (base32upper),
  base32z: () => (base32z)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base36.js
var base36_namespaceObject = {};
__webpack_require__.r(base36_namespaceObject);
__webpack_require__.d(base36_namespaceObject, {
  base36: () => (base36),
  base36upper: () => (base36upper)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base58.js
var base58_namespaceObject = {};
__webpack_require__.r(base58_namespaceObject);
__webpack_require__.d(base58_namespaceObject, {
  base58btc: () => (base58btc),
  base58flickr: () => (base58flickr)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base64.js
var base64_namespaceObject = {};
__webpack_require__.r(base64_namespaceObject);
__webpack_require__.d(base64_namespaceObject, {
  base64: () => (base64),
  base64pad: () => (base64pad),
  base64url: () => (base64url),
  base64urlpad: () => (base64urlpad)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base8.js
var base8_namespaceObject = {};
__webpack_require__.r(base8_namespaceObject);
__webpack_require__.d(base8_namespaceObject, {
  base8: () => (base8)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/identity.js
var identity_namespaceObject = {};
__webpack_require__.r(identity_namespaceObject);
__webpack_require__.d(identity_namespaceObject, {
  identity: () => (identity)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/codecs/json.js
var json_namespaceObject = {};
__webpack_require__.r(json_namespaceObject);
__webpack_require__.d(json_namespaceObject, {
  code: () => (code),
  decode: () => (json_decode),
  encode: () => (json_encode),
  name: () => (json_name)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/codecs/raw.js
var raw_namespaceObject = {};
__webpack_require__.r(raw_namespaceObject);
__webpack_require__.d(raw_namespaceObject, {
  code: () => (raw_code),
  decode: () => (raw_decode),
  encode: () => (raw_encode),
  name: () => (raw_name)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/hashes/identity.js
var hashes_identity_namespaceObject = {};
__webpack_require__.r(hashes_identity_namespaceObject);
__webpack_require__.d(hashes_identity_namespaceObject, {
  identity: () => (identity_identity)
});

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/hashes/sha2-browser.js
var sha2_browser_namespaceObject = {};
__webpack_require__.r(sha2_browser_namespaceObject);
__webpack_require__.d(sha2_browser_namespaceObject, {
  sha256: () => (sha256),
  sha512: () => (sha512)
});

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/it-ws@6.1.5/node_modules/it-ws/dist/src/ready.js
/* harmony default export */ const ready = (async (socket) => {
    // if the socket is closing or closed, return end
    if (socket.readyState >= 2) {
        throw new Error('socket closed');
    }
    // if open, return
    if (socket.readyState === 1) {
        return;
    }
    await new Promise((resolve, reject) => {
        function cleanup() {
            socket.removeEventListener('open', handleOpen);
            socket.removeEventListener('error', handleErr);
        }
        function handleOpen() {
            cleanup();
            resolve();
        }
        function handleErr(event) {
            cleanup();
            reject(event.error ?? new Error(`connect ECONNREFUSED ${socket.url}`));
        }
        socket.addEventListener('open', handleOpen);
        socket.addEventListener('error', handleErr);
    });
});
//# sourceMappingURL=ready.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/it-ws@6.1.5/node_modules/it-ws/dist/src/sink.js

/* harmony default export */ const sink = ((socket, options) => {
    options = options ?? {};
    options.closeOnEnd = options.closeOnEnd !== false;
    const sink = async (source) => {
        for await (const data of source) {
            try {
                await ready(socket);
            }
            catch (err) {
                if (err.message === 'socket closed')
                    break;
                throw err;
            }
            // the ready promise resolved without error but the socket was closing so
            // exit the loop and don't send data
            if (socket.readyState === socket.CLOSING || socket.readyState === socket.CLOSED) {
                break;
            }
            socket.send(data);
        }
        if (options.closeOnEnd != null && socket.readyState <= 1) {
            await new Promise((resolve, reject) => {
                socket.addEventListener('close', event => {
                    if (event.wasClean || event.code === 1006) {
                        resolve();
                    }
                    else {
                        const err = Object.assign(new Error('ws error'), { event });
                        reject(err);
                    }
                });
                setTimeout(() => { socket.close(); });
            });
        }
    };
    return sink;
});
//# sourceMappingURL=sink.js.map
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/event-iterator@2.0.0/node_modules/event-iterator/lib/dom.js
var dom = __webpack_require__(87679);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bytes.js
const empty = new Uint8Array(0);
function toHex(d) {
    return d.reduce((hex, byte) => hex + byte.toString(16).padStart(2, '0'), '');
}
function fromHex(hex) {
    const hexes = hex.match(/../g);
    return hexes != null ? new Uint8Array(hexes.map(b => parseInt(b, 16))) : empty;
}
function equals(aa, bb) {
    if (aa === bb) {
        return true;
    }
    if (aa.byteLength !== bb.byteLength) {
        return false;
    }
    for (let ii = 0; ii < aa.byteLength; ii++) {
        if (aa[ii] !== bb[ii]) {
            return false;
        }
    }
    return true;
}
function coerce(o) {
    if (o instanceof Uint8Array && o.constructor.name === 'Uint8Array') {
        return o;
    }
    if (o instanceof ArrayBuffer) {
        return new Uint8Array(o);
    }
    if (ArrayBuffer.isView(o)) {
        return new Uint8Array(o.buffer, o.byteOffset, o.byteLength);
    }
    throw new Error('Unknown type, must be binary type');
}
function isBinary(o) {
    return o instanceof ArrayBuffer || ArrayBuffer.isView(o);
}
function fromString(str) {
    return new TextEncoder().encode(str);
}
function bytes_toString(b) {
    return new TextDecoder().decode(b);
}
//# sourceMappingURL=bytes.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/vendor/base-x.js
/* eslint-disable */
// base-x encoding / decoding
// Copyright (c) 2018 base-x contributors
// Copyright (c) 2014-2018 The Bitcoin Core developers (base58.cpp)
// Distributed under the MIT software license, see the accompanying
// file LICENSE or http://www.opensource.org/licenses/mit-license.php.
/**
 * @param {string} ALPHABET
 * @param {any} name
 */
function base(ALPHABET, name) {
    if (ALPHABET.length >= 255) {
        throw new TypeError('Alphabet too long');
    }
    var BASE_MAP = new Uint8Array(256);
    for (var j = 0; j < BASE_MAP.length; j++) {
        BASE_MAP[j] = 255;
    }
    for (var i = 0; i < ALPHABET.length; i++) {
        var x = ALPHABET.charAt(i);
        var xc = x.charCodeAt(0);
        if (BASE_MAP[xc] !== 255) {
            throw new TypeError(x + ' is ambiguous');
        }
        BASE_MAP[xc] = i;
    }
    var BASE = ALPHABET.length;
    var LEADER = ALPHABET.charAt(0);
    var FACTOR = Math.log(BASE) / Math.log(256); // log(BASE) / log(256), rounded up
    var iFACTOR = Math.log(256) / Math.log(BASE); // log(256) / log(BASE), rounded up
    /**
     * @param {any[] | Iterable<number>} source
     */
    function encode(source) {
        // @ts-ignore
        if (source instanceof Uint8Array)
            ;
        else if (ArrayBuffer.isView(source)) {
            source = new Uint8Array(source.buffer, source.byteOffset, source.byteLength);
        }
        else if (Array.isArray(source)) {
            source = Uint8Array.from(source);
        }
        if (!(source instanceof Uint8Array)) {
            throw new TypeError('Expected Uint8Array');
        }
        if (source.length === 0) {
            return '';
        }
        // Skip & count leading zeroes.
        var zeroes = 0;
        var length = 0;
        var pbegin = 0;
        var pend = source.length;
        while (pbegin !== pend && source[pbegin] === 0) {
            pbegin++;
            zeroes++;
        }
        // Allocate enough space in big-endian base58 representation.
        var size = ((pend - pbegin) * iFACTOR + 1) >>> 0;
        var b58 = new Uint8Array(size);
        // Process the bytes.
        while (pbegin !== pend) {
            var carry = source[pbegin];
            // Apply "b58 = b58 * 256 + ch".
            var i = 0;
            for (var it1 = size - 1; (carry !== 0 || i < length) && (it1 !== -1); it1--, i++) {
                carry += (256 * b58[it1]) >>> 0;
                b58[it1] = (carry % BASE) >>> 0;
                carry = (carry / BASE) >>> 0;
            }
            if (carry !== 0) {
                throw new Error('Non-zero carry');
            }
            length = i;
            pbegin++;
        }
        // Skip leading zeroes in base58 result.
        var it2 = size - length;
        while (it2 !== size && b58[it2] === 0) {
            it2++;
        }
        // Translate the result into a string.
        var str = LEADER.repeat(zeroes);
        for (; it2 < size; ++it2) {
            str += ALPHABET.charAt(b58[it2]);
        }
        return str;
    }
    /**
     * @param {string | string[]} source
     */
    function decodeUnsafe(source) {
        if (typeof source !== 'string') {
            throw new TypeError('Expected String');
        }
        if (source.length === 0) {
            return new Uint8Array();
        }
        var psz = 0;
        // Skip leading spaces.
        if (source[psz] === ' ') {
            return;
        }
        // Skip and count leading '1's.
        var zeroes = 0;
        var length = 0;
        while (source[psz] === LEADER) {
            zeroes++;
            psz++;
        }
        // Allocate enough space in big-endian base256 representation.
        var size = (((source.length - psz) * FACTOR) + 1) >>> 0; // log(58) / log(256), rounded up.
        var b256 = new Uint8Array(size);
        // Process the characters.
        while (source[psz]) {
            // Decode character
            var carry = BASE_MAP[source.charCodeAt(psz)];
            // Invalid character
            if (carry === 255) {
                return;
            }
            var i = 0;
            for (var it3 = size - 1; (carry !== 0 || i < length) && (it3 !== -1); it3--, i++) {
                carry += (BASE * b256[it3]) >>> 0;
                b256[it3] = (carry % 256) >>> 0;
                carry = (carry / 256) >>> 0;
            }
            if (carry !== 0) {
                throw new Error('Non-zero carry');
            }
            length = i;
            psz++;
        }
        // Skip trailing spaces.
        if (source[psz] === ' ') {
            return;
        }
        // Skip leading zeroes in b256.
        var it4 = size - length;
        while (it4 !== size && b256[it4] === 0) {
            it4++;
        }
        var vch = new Uint8Array(zeroes + (size - it4));
        var j = zeroes;
        while (it4 !== size) {
            vch[j++] = b256[it4++];
        }
        return vch;
    }
    /**
     * @param {string | string[]} string
     */
    function decode(string) {
        var buffer = decodeUnsafe(string);
        if (buffer) {
            return buffer;
        }
        throw new Error(`Non-${name} character`);
    }
    return {
        encode: encode,
        decodeUnsafe: decodeUnsafe,
        decode: decode
    };
}
var src = base;
var _brrp__multiformats_scope_baseX = src;
/* harmony default export */ const base_x = (_brrp__multiformats_scope_baseX);
//# sourceMappingURL=base-x.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base.js


/**
 * Class represents both BaseEncoder and MultibaseEncoder meaning it
 * can be used to encode to multibase or base encode without multibase
 * prefix.
 */
class Encoder {
    name;
    prefix;
    baseEncode;
    constructor(name, prefix, baseEncode) {
        this.name = name;
        this.prefix = prefix;
        this.baseEncode = baseEncode;
    }
    encode(bytes) {
        if (bytes instanceof Uint8Array) {
            return `${this.prefix}${this.baseEncode(bytes)}`;
        }
        else {
            throw Error('Unknown type, must be binary type');
        }
    }
}
/**
 * Class represents both BaseDecoder and MultibaseDecoder so it could be used
 * to decode multibases (with matching prefix) or just base decode strings
 * with corresponding base encoding.
 */
class Decoder {
    name;
    prefix;
    baseDecode;
    prefixCodePoint;
    constructor(name, prefix, baseDecode) {
        this.name = name;
        this.prefix = prefix;
        const prefixCodePoint = prefix.codePointAt(0);
        /* c8 ignore next 3 */
        if (prefixCodePoint === undefined) {
            throw new Error('Invalid prefix character');
        }
        this.prefixCodePoint = prefixCodePoint;
        this.baseDecode = baseDecode;
    }
    decode(text) {
        if (typeof text === 'string') {
            if (text.codePointAt(0) !== this.prefixCodePoint) {
                throw Error(`Unable to decode multibase string ${JSON.stringify(text)}, ${this.name} decoder only supports inputs prefixed with ${this.prefix}`);
            }
            return this.baseDecode(text.slice(this.prefix.length));
        }
        else {
            throw Error('Can only multibase decode strings');
        }
    }
    or(decoder) {
        return or(this, decoder);
    }
}
class ComposedDecoder {
    decoders;
    constructor(decoders) {
        this.decoders = decoders;
    }
    or(decoder) {
        return or(this, decoder);
    }
    decode(input) {
        const prefix = input[0];
        const decoder = this.decoders[prefix];
        if (decoder != null) {
            return decoder.decode(input);
        }
        else {
            throw RangeError(`Unable to decode multibase string ${JSON.stringify(input)}, only inputs prefixed with ${Object.keys(this.decoders)} are supported`);
        }
    }
}
function or(left, right) {
    return new ComposedDecoder({
        ...(left.decoders ?? { [left.prefix]: left }),
        ...(right.decoders ?? { [right.prefix]: right })
    });
}
class Codec {
    name;
    prefix;
    baseEncode;
    baseDecode;
    encoder;
    decoder;
    constructor(name, prefix, baseEncode, baseDecode) {
        this.name = name;
        this.prefix = prefix;
        this.baseEncode = baseEncode;
        this.baseDecode = baseDecode;
        this.encoder = new Encoder(name, prefix, baseEncode);
        this.decoder = new Decoder(name, prefix, baseDecode);
    }
    encode(input) {
        return this.encoder.encode(input);
    }
    decode(input) {
        return this.decoder.decode(input);
    }
}
function from({ name, prefix, encode, decode }) {
    return new Codec(name, prefix, encode, decode);
}
function baseX({ name, prefix, alphabet }) {
    const { encode, decode } = base_x(alphabet, name);
    return from({
        prefix,
        name,
        encode,
        decode: (text) => coerce(decode(text))
    });
}
function decode(string, alphabetIdx, bitsPerChar, name) {
    // Count the padding bytes:
    let end = string.length;
    while (string[end - 1] === '=') {
        --end;
    }
    // Allocate the output:
    const out = new Uint8Array((end * bitsPerChar / 8) | 0);
    // Parse the data:
    let bits = 0; // Number of bits currently in the buffer
    let buffer = 0; // Bits waiting to be written out, MSB first
    let written = 0; // Next byte to write
    for (let i = 0; i < end; ++i) {
        // Read one character from the string:
        const value = alphabetIdx[string[i]];
        if (value === undefined) {
            throw new SyntaxError(`Non-${name} character`);
        }
        // Append the bits to the buffer:
        buffer = (buffer << bitsPerChar) | value;
        bits += bitsPerChar;
        // Write out some bits if the buffer has a byte's worth:
        if (bits >= 8) {
            bits -= 8;
            out[written++] = 0xff & (buffer >> bits);
        }
    }
    // Verify that we have received just enough bits:
    if (bits >= bitsPerChar || (0xff & (buffer << (8 - bits))) !== 0) {
        throw new SyntaxError('Unexpected end of data');
    }
    return out;
}
function encode(data, alphabet, bitsPerChar) {
    const pad = alphabet[alphabet.length - 1] === '=';
    const mask = (1 << bitsPerChar) - 1;
    let out = '';
    let bits = 0; // Number of bits currently in the buffer
    let buffer = 0; // Bits waiting to be written out, MSB first
    for (let i = 0; i < data.length; ++i) {
        // Slurp data into the buffer:
        buffer = (buffer << 8) | data[i];
        bits += 8;
        // Write out as much as we can:
        while (bits > bitsPerChar) {
            bits -= bitsPerChar;
            out += alphabet[mask & (buffer >> bits)];
        }
    }
    // Partial character:
    if (bits !== 0) {
        out += alphabet[mask & (buffer << (bitsPerChar - bits))];
    }
    // Add padding characters until we hit a byte boundary:
    if (pad) {
        while (((out.length * bitsPerChar) & 7) !== 0) {
            out += '=';
        }
    }
    return out;
}
function createAlphabetIdx(alphabet) {
    // Build the character lookup table:
    const alphabetIdx = {};
    for (let i = 0; i < alphabet.length; ++i) {
        alphabetIdx[alphabet[i]] = i;
    }
    return alphabetIdx;
}
/**
 * RFC4648 Factory
 */
function rfc4648({ name, prefix, bitsPerChar, alphabet }) {
    const alphabetIdx = createAlphabetIdx(alphabet);
    return from({
        prefix,
        name,
        encode(input) {
            return encode(input, alphabet, bitsPerChar);
        },
        decode(input) {
            return decode(input, alphabetIdx, bitsPerChar, name);
        }
    });
}
//# sourceMappingURL=base.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base10.js

const base10 = baseX({
    prefix: '9',
    name: 'base10',
    alphabet: '0123456789'
});
//# sourceMappingURL=base10.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base16.js

const base16 = rfc4648({
    prefix: 'f',
    name: 'base16',
    alphabet: '0123456789abcdef',
    bitsPerChar: 4
});
const base16upper = rfc4648({
    prefix: 'F',
    name: 'base16upper',
    alphabet: '0123456789ABCDEF',
    bitsPerChar: 4
});
//# sourceMappingURL=base16.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base2.js

const base2 = rfc4648({
    prefix: '0',
    name: 'base2',
    alphabet: '01',
    bitsPerChar: 1
});
//# sourceMappingURL=base2.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base256emoji.js

const alphabet = Array.from('🚀🪐☄🛰🌌🌑🌒🌓🌔🌕🌖🌗🌘🌍🌏🌎🐉☀💻🖥💾💿😂❤😍🤣😊🙏💕😭😘👍😅👏😁🔥🥰💔💖💙😢🤔😆🙄💪😉☺👌🤗💜😔😎😇🌹🤦🎉💞✌✨🤷😱😌🌸🙌😋💗💚😏💛🙂💓🤩😄😀🖤😃💯🙈👇🎶😒🤭❣😜💋👀😪😑💥🙋😞😩😡🤪👊🥳😥🤤👉💃😳✋😚😝😴🌟😬🙃🍀🌷😻😓⭐✅🥺🌈😈🤘💦✔😣🏃💐☹🎊💘😠☝😕🌺🎂🌻😐🖕💝🙊😹🗣💫💀👑🎵🤞😛🔴😤🌼😫⚽🤙☕🏆🤫👈😮🙆🍻🍃🐶💁😲🌿🧡🎁⚡🌞🎈❌✊👋😰🤨😶🤝🚶💰🍓💢🤟🙁🚨💨🤬✈🎀🍺🤓😙💟🌱😖👶🥴▶➡❓💎💸⬇😨🌚🦋😷🕺⚠🙅😟😵👎🤲🤠🤧📌🔵💅🧐🐾🍒😗🤑🌊🤯🐷☎💧😯💆👆🎤🙇🍑❄🌴💣🐸💌📍🥀🤢👅💡💩👐📸👻🤐🤮🎼🥵🚩🍎🍊👼💍📣🥂');
const alphabetBytesToChars = (alphabet.reduce((p, c, i) => { p[i] = c; return p; }, ([])));
const alphabetCharsToBytes = (alphabet.reduce((p, c, i) => {
    const codePoint = c.codePointAt(0);
    if (codePoint == null) {
        throw new Error(`Invalid character: ${c}`);
    }
    p[codePoint] = i;
    return p;
}, ([])));
function base256emoji_encode(data) {
    return data.reduce((p, c) => {
        p += alphabetBytesToChars[c];
        return p;
    }, '');
}
function base256emoji_decode(str) {
    const byts = [];
    for (const char of str) {
        const codePoint = char.codePointAt(0);
        if (codePoint == null) {
            throw new Error(`Invalid character: ${char}`);
        }
        const byt = alphabetCharsToBytes[codePoint];
        if (byt == null) {
            throw new Error(`Non-base256emoji character: ${char}`);
        }
        byts.push(byt);
    }
    return new Uint8Array(byts);
}
const base256emoji = from({
    prefix: '🚀',
    name: 'base256emoji',
    encode: base256emoji_encode,
    decode: base256emoji_decode
});
//# sourceMappingURL=base256emoji.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base32.js

const base32 = rfc4648({
    prefix: 'b',
    name: 'base32',
    alphabet: 'abcdefghijklmnopqrstuvwxyz234567',
    bitsPerChar: 5
});
const base32upper = rfc4648({
    prefix: 'B',
    name: 'base32upper',
    alphabet: 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567',
    bitsPerChar: 5
});
const base32pad = rfc4648({
    prefix: 'c',
    name: 'base32pad',
    alphabet: 'abcdefghijklmnopqrstuvwxyz234567=',
    bitsPerChar: 5
});
const base32padupper = rfc4648({
    prefix: 'C',
    name: 'base32padupper',
    alphabet: 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567=',
    bitsPerChar: 5
});
const base32hex = rfc4648({
    prefix: 'v',
    name: 'base32hex',
    alphabet: '0123456789abcdefghijklmnopqrstuv',
    bitsPerChar: 5
});
const base32hexupper = rfc4648({
    prefix: 'V',
    name: 'base32hexupper',
    alphabet: '0123456789ABCDEFGHIJKLMNOPQRSTUV',
    bitsPerChar: 5
});
const base32hexpad = rfc4648({
    prefix: 't',
    name: 'base32hexpad',
    alphabet: '0123456789abcdefghijklmnopqrstuv=',
    bitsPerChar: 5
});
const base32hexpadupper = rfc4648({
    prefix: 'T',
    name: 'base32hexpadupper',
    alphabet: '0123456789ABCDEFGHIJKLMNOPQRSTUV=',
    bitsPerChar: 5
});
const base32z = rfc4648({
    prefix: 'h',
    name: 'base32z',
    alphabet: 'ybndrfg8ejkmcpqxot1uwisza345h769',
    bitsPerChar: 5
});
//# sourceMappingURL=base32.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base36.js

const base36 = baseX({
    prefix: 'k',
    name: 'base36',
    alphabet: '0123456789abcdefghijklmnopqrstuvwxyz'
});
const base36upper = baseX({
    prefix: 'K',
    name: 'base36upper',
    alphabet: '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ'
});
//# sourceMappingURL=base36.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base58.js

const base58btc = baseX({
    name: 'base58btc',
    prefix: 'z',
    alphabet: '123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz'
});
const base58flickr = baseX({
    name: 'base58flickr',
    prefix: 'Z',
    alphabet: '123456789abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ'
});
//# sourceMappingURL=base58.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base64.js

const base64 = rfc4648({
    prefix: 'm',
    name: 'base64',
    alphabet: 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/',
    bitsPerChar: 6
});
const base64pad = rfc4648({
    prefix: 'M',
    name: 'base64pad',
    alphabet: 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=',
    bitsPerChar: 6
});
const base64url = rfc4648({
    prefix: 'u',
    name: 'base64url',
    alphabet: 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_',
    bitsPerChar: 6
});
const base64urlpad = rfc4648({
    prefix: 'U',
    name: 'base64urlpad',
    alphabet: 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_=',
    bitsPerChar: 6
});
//# sourceMappingURL=base64.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/base8.js

const base8 = rfc4648({
    prefix: '7',
    name: 'base8',
    alphabet: '01234567',
    bitsPerChar: 3
});
//# sourceMappingURL=base8.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/bases/identity.js


const identity = from({
    prefix: '\x00',
    name: 'identity',
    encode: (buf) => bytes_toString(buf),
    decode: (str) => fromString(str)
});
//# sourceMappingURL=identity.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/codecs/json.js
const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder();
const json_name = 'json';
const code = 0x0200;
function json_encode(node) {
    return textEncoder.encode(JSON.stringify(node));
}
function json_decode(data) {
    return JSON.parse(textDecoder.decode(data));
}
//# sourceMappingURL=json.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/codecs/raw.js

const raw_name = 'raw';
const raw_code = 0x55;
function raw_encode(node) {
    return coerce(node);
}
function raw_decode(data) {
    return coerce(data);
}
//# sourceMappingURL=raw.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/vendor/varint.js
/* eslint-disable */
var encode_1 = varint_encode;
var MSB = 0x80, REST = 0x7F, MSBALL = ~REST, INT = Math.pow(2, 31);
/**
 * @param {number} num
 * @param {number[]} out
 * @param {number} offset
 */
function varint_encode(num, out, offset) {
    out = out || [];
    offset = offset || 0;
    var oldOffset = offset;
    while (num >= INT) {
        out[offset++] = (num & 0xFF) | MSB;
        num /= 128;
    }
    while (num & MSBALL) {
        out[offset++] = (num & 0xFF) | MSB;
        num >>>= 7;
    }
    out[offset] = num | 0;
    // @ts-ignore
    varint_encode.bytes = offset - oldOffset + 1;
    return out;
}
var varint_decode = read;
var MSB$1 = 0x80, REST$1 = 0x7F;
/**
 * @param {string | any[]} buf
 * @param {number} offset
 */
function read(buf, offset) {
    var res = 0, offset = offset || 0, shift = 0, counter = offset, b, l = buf.length;
    do {
        if (counter >= l) {
            // @ts-ignore
            read.bytes = 0;
            throw new RangeError('Could not decode varint');
        }
        b = buf[counter++];
        res += shift < 28
            ? (b & REST$1) << shift
            : (b & REST$1) * Math.pow(2, shift);
        shift += 7;
    } while (b >= MSB$1);
    // @ts-ignore
    read.bytes = counter - offset;
    return res;
}
var N1 = Math.pow(2, 7);
var N2 = Math.pow(2, 14);
var N3 = Math.pow(2, 21);
var N4 = Math.pow(2, 28);
var N5 = Math.pow(2, 35);
var N6 = Math.pow(2, 42);
var N7 = Math.pow(2, 49);
var N8 = Math.pow(2, 56);
var N9 = Math.pow(2, 63);
var varint_length = function (/** @type {number} */ value) {
    return (value < N1 ? 1
        : value < N2 ? 2
            : value < N3 ? 3
                : value < N4 ? 4
                    : value < N5 ? 5
                        : value < N6 ? 6
                            : value < N7 ? 7
                                : value < N8 ? 8
                                    : value < N9 ? 9
                                        : 10);
};
var varint = {
    encode: encode_1,
    decode: varint_decode,
    encodingLength: varint_length
};
var _brrp_varint = varint;
/* harmony default export */ const vendor_varint = (_brrp_varint);
//# sourceMappingURL=varint.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/varint.js

function src_varint_decode(data, offset = 0) {
    const code = vendor_varint.decode(data, offset);
    return [code, vendor_varint.decode.bytes];
}
function encodeTo(int, target, offset = 0) {
    vendor_varint.encode(int, target, offset);
    return target;
}
function encodingLength(int) {
    return vendor_varint.encodingLength(int);
}
//# sourceMappingURL=varint.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/hashes/digest.js


/**
 * Creates a multihash digest.
 */
function create(code, digest) {
    const size = digest.byteLength;
    const sizeOffset = encodingLength(code);
    const digestOffset = sizeOffset + encodingLength(size);
    const bytes = new Uint8Array(digestOffset + size);
    encodeTo(code, bytes, 0);
    encodeTo(size, bytes, sizeOffset);
    bytes.set(digest, digestOffset);
    return new Digest(code, size, digest, bytes);
}
/**
 * Turns bytes representation of multihash digest into an instance.
 */
function digest_decode(multihash) {
    const bytes = coerce(multihash);
    const [code, sizeOffset] = src_varint_decode(bytes);
    const [size, digestOffset] = src_varint_decode(bytes.subarray(sizeOffset));
    const digest = bytes.subarray(sizeOffset + digestOffset);
    if (digest.byteLength !== size) {
        throw new Error('Incorrect length');
    }
    return new Digest(code, size, digest, bytes);
}
function digest_equals(a, b) {
    if (a === b) {
        return true;
    }
    else {
        const data = b;
        return (a.code === data.code &&
            a.size === data.size &&
            data.bytes instanceof Uint8Array &&
            equals(a.bytes, data.bytes));
    }
}
/**
 * Represents a multihash digest which carries information about the
 * hashing algorithm and an actual hash digest.
 */
class Digest {
    code;
    size;
    digest;
    bytes;
    /**
     * Creates a multihash digest.
     */
    constructor(code, size, digest, bytes) {
        this.code = code;
        this.size = size;
        this.digest = digest;
        this.bytes = bytes;
    }
}
/**
 * Used to check that the passed multihash has the passed code
 */
function hasCode(digest, code) {
    return digest.code === code;
}
//# sourceMappingURL=digest.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/hashes/identity.js


const identity_code = 0x0;
const identity_name = 'identity';
const identity_encode = coerce;
function digest(input, options) {
    if (options?.truncate != null && options.truncate !== input.byteLength) {
        if (options.truncate < 0 || options.truncate > input.byteLength) {
            throw new Error(`Invalid truncate option, must be less than or equal to ${input.byteLength}`);
        }
        input = input.subarray(0, options.truncate);
    }
    return create(identity_code, identity_encode(input));
}
const identity_identity = { code: identity_code, name: identity_name, encode: identity_encode, digest };
//# sourceMappingURL=identity.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/hashes/hasher.js

const DEFAULT_MIN_DIGEST_LENGTH = 20;
function hasher_from({ name, code, encode, minDigestLength, maxDigestLength }) {
    return new Hasher(name, code, encode, minDigestLength, maxDigestLength);
}
/**
 * Hasher represents a hashing algorithm implementation that produces as
 * `MultihashDigest`.
 */
class Hasher {
    name;
    code;
    encode;
    minDigestLength;
    maxDigestLength;
    constructor(name, code, encode, minDigestLength, maxDigestLength) {
        this.name = name;
        this.code = code;
        this.encode = encode;
        this.minDigestLength = minDigestLength ?? DEFAULT_MIN_DIGEST_LENGTH;
        this.maxDigestLength = maxDigestLength;
    }
    digest(input, options) {
        if (options?.truncate != null) {
            if (options.truncate < this.minDigestLength) {
                throw new Error(`Invalid truncate option, must be greater than or equal to ${this.minDigestLength}`);
            }
            if (this.maxDigestLength != null && options.truncate > this.maxDigestLength) {
                throw new Error(`Invalid truncate option, must be less than or equal to ${this.maxDigestLength}`);
            }
        }
        if (input instanceof Uint8Array) {
            const result = this.encode(input);
            if (result instanceof Uint8Array) {
                return createDigest(result, this.code, options?.truncate);
            }
            return result.then(digest => createDigest(digest, this.code, options?.truncate));
        }
        else {
            throw Error('Unknown type, must be binary type');
            /* c8 ignore next 1 */
        }
    }
}
/**
 * Create a Digest from the passed uint8array and code, optionally truncating it
 * first.
 */
function createDigest(digest, code, truncate) {
    if (truncate != null && truncate !== digest.byteLength) {
        if (truncate > digest.byteLength) {
            throw new Error(`Invalid truncate option, must be less than or equal to ${digest.byteLength}`);
        }
        digest = digest.subarray(0, truncate);
    }
    return create(code, digest);
}
//# sourceMappingURL=hasher.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/hashes/sha2-browser.js
/* global crypto */

function sha(name) {
    return async (data) => new Uint8Array(await crypto.subtle.digest(name, data));
}
const sha256 = hasher_from({
    name: 'sha2-256',
    code: 0x12,
    encode: sha('SHA-256')
});
const sha512 = hasher_from({
    name: 'sha2-512',
    code: 0x13,
    encode: sha('SHA-512')
});
//# sourceMappingURL=sha2-browser.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/cid.js






// This way TS will also expose all the types from module

function format(link, base) {
    const { bytes, version } = link;
    switch (version) {
        case 0:
            return toStringV0(bytes, baseCache(link), base ?? base58btc.encoder);
        default:
            return toStringV1(bytes, baseCache(link), (base ?? base32.encoder));
    }
}
function toJSON(link) {
    return {
        '/': format(link)
    };
}
function fromJSON(json) {
    return CID.parse(json['/']);
}
const cache = new WeakMap();
function baseCache(cid) {
    const baseCache = cache.get(cid);
    if (baseCache == null) {
        const baseCache = new Map();
        cache.set(cid, baseCache);
        return baseCache;
    }
    return baseCache;
}
class CID {
    code;
    version;
    multihash;
    bytes;
    '/';
    /**
     * @param version - Version of the CID
     * @param code - Code of the codec content is encoded in, see https://github.com/multiformats/multicodec/blob/master/table.csv
     * @param multihash - (Multi)hash of the of the content.
     */
    constructor(version, code, multihash, bytes) {
        this.code = code;
        this.version = version;
        this.multihash = multihash;
        this.bytes = bytes;
        // flag to serializers that this is a CID and
        // should be treated specially
        this['/'] = bytes;
    }
    /**
     * Signalling `cid.asCID === cid` has been replaced with `cid['/'] === cid.bytes`
     * please either use `CID.asCID(cid)` or switch to new signalling mechanism
     *
     * @deprecated
     */
    get asCID() {
        return this;
    }
    // ArrayBufferView
    get byteOffset() {
        return this.bytes.byteOffset;
    }
    // ArrayBufferView
    get byteLength() {
        return this.bytes.byteLength;
    }
    toV0() {
        switch (this.version) {
            case 0: {
                return this;
            }
            case 1: {
                const { code, multihash } = this;
                if (code !== DAG_PB_CODE) {
                    throw new Error('Cannot convert a non dag-pb CID to CIDv0');
                }
                // sha2-256
                if (multihash.code !== SHA_256_CODE) {
                    throw new Error('Cannot convert non sha2-256 multihash CID to CIDv0');
                }
                return (CID.createV0(multihash));
            }
            default: {
                throw Error(`Can not convert CID version ${this.version} to version 0. This is a bug please report`);
            }
        }
    }
    toV1() {
        switch (this.version) {
            case 0: {
                const { code, digest } = this.multihash;
                const multihash = create(code, digest);
                return (CID.createV1(this.code, multihash));
            }
            case 1: {
                return this;
            }
            default: {
                throw Error(`Can not convert CID version ${this.version} to version 1. This is a bug please report`);
            }
        }
    }
    equals(other) {
        return CID.equals(this, other);
    }
    static equals(self, other) {
        const unknown = other;
        return (unknown != null &&
            self.code === unknown.code &&
            self.version === unknown.version &&
            digest_equals(self.multihash, unknown.multihash));
    }
    toString(base) {
        return format(this, base);
    }
    toJSON() {
        return { '/': format(this) };
    }
    link() {
        return this;
    }
    [Symbol.toStringTag] = 'CID';
    // Legacy
    [Symbol.for('nodejs.util.inspect.custom')]() {
        return `CID(${this.toString()})`;
    }
    /**
     * Takes any input `value` and returns a `CID` instance if it was
     * a `CID` otherwise returns `null`. If `value` is instanceof `CID`
     * it will return value back. If `value` is not instance of this CID
     * class, but is compatible CID it will return new instance of this
     * `CID` class. Otherwise returns null.
     *
     * This allows two different incompatible versions of CID library to
     * co-exist and interop as long as binary interface is compatible.
     */
    static asCID(input) {
        if (input == null) {
            return null;
        }
        const value = input;
        if (value instanceof CID) {
            // If value is instance of CID then we're all set.
            return value;
        }
        else if ((value['/'] != null && value['/'] === value.bytes) || value.asCID === value) {
            // If value isn't instance of this CID class but `this.asCID === this` or
            // `value['/'] === value.bytes` is true it is CID instance coming from a
            // different implementation (diff version or duplicate). In that case we
            // rebase it to this `CID` implementation so caller is guaranteed to get
            // instance with expected API.
            const { version, code, multihash, bytes } = value;
            return new CID(version, code, multihash, bytes ?? encodeCID(version, code, multihash.bytes));
        }
        else if (value[cidSymbol] === true) {
            // If value is a CID from older implementation that used to be tagged via
            // symbol we still rebase it to the this `CID` implementation by
            // delegating that to a constructor.
            const { version, multihash, code } = value;
            const digest = digest_decode(multihash);
            return CID.create(version, code, digest);
        }
        else {
            // Otherwise value is not a CID (or an incompatible version of it) in
            // which case we return `null`.
            return null;
        }
    }
    /**
     * @param version - Version of the CID
     * @param code - Code of the codec content is encoded in, see https://github.com/multiformats/multicodec/blob/master/table.csv
     * @param digest - (Multi)hash of the of the content.
     */
    static create(version, code, digest) {
        if (typeof code !== 'number') {
            throw new Error('String codecs are no longer supported');
        }
        if (!(digest.bytes instanceof Uint8Array)) {
            throw new Error('Invalid digest');
        }
        switch (version) {
            case 0: {
                if (code !== DAG_PB_CODE) {
                    throw new Error(`Version 0 CID must use dag-pb (code: ${DAG_PB_CODE}) block encoding`);
                }
                else {
                    return new CID(version, code, digest, digest.bytes);
                }
            }
            case 1: {
                const bytes = encodeCID(version, code, digest.bytes);
                return new CID(version, code, digest, bytes);
            }
            default: {
                throw new Error('Invalid version');
            }
        }
    }
    /**
     * Simplified version of `create` for CIDv0.
     */
    static createV0(digest) {
        return CID.create(0, DAG_PB_CODE, digest);
    }
    /**
     * Simplified version of `create` for CIDv1.
     *
     * @param code - Content encoding format code.
     * @param digest - Multihash of the content.
     */
    static createV1(code, digest) {
        return CID.create(1, code, digest);
    }
    /**
     * Decoded a CID from its binary representation. The byte array must contain
     * only the CID with no additional bytes.
     *
     * An error will be thrown if the bytes provided do not contain a valid
     * binary representation of a CID.
     */
    static decode(bytes) {
        const [cid, remainder] = CID.decodeFirst(bytes);
        if (remainder.length !== 0) {
            throw new Error('Incorrect length');
        }
        return cid;
    }
    /**
     * Decoded a CID from its binary representation at the beginning of a byte
     * array.
     *
     * Returns an array with the first element containing the CID and the second
     * element containing the remainder of the original byte array. The remainder
     * will be a zero-length byte array if the provided bytes only contained a
     * binary CID representation.
     */
    static decodeFirst(bytes) {
        const specs = CID.inspectBytes(bytes);
        const prefixSize = specs.size - specs.multihashSize;
        const multihashBytes = coerce(bytes.subarray(prefixSize, prefixSize + specs.multihashSize));
        if (multihashBytes.byteLength !== specs.multihashSize) {
            throw new Error('Incorrect length');
        }
        const digestBytes = multihashBytes.subarray(specs.multihashSize - specs.digestSize);
        const digest = new Digest(specs.multihashCode, specs.digestSize, digestBytes, multihashBytes);
        const cid = specs.version === 0
            ? CID.createV0(digest)
            : CID.createV1(specs.codec, digest);
        return [cid, bytes.subarray(specs.size)];
    }
    /**
     * Inspect the initial bytes of a CID to determine its properties.
     *
     * Involves decoding up to 4 varints. Typically this will require only 4 to 6
     * bytes but for larger multicodec code values and larger multihash digest
     * lengths these varints can be quite large. It is recommended that at least
     * 10 bytes be made available in the `initialBytes` argument for a complete
     * inspection.
     */
    static inspectBytes(initialBytes) {
        let offset = 0;
        const next = () => {
            const [i, length] = src_varint_decode(initialBytes.subarray(offset));
            offset += length;
            return i;
        };
        let version = next();
        let codec = DAG_PB_CODE;
        if (version === 18) {
            // CIDv0
            version = 0;
            offset = 0;
        }
        else {
            codec = next();
        }
        if (version !== 0 && version !== 1) {
            throw new RangeError(`Invalid CID version ${version}`);
        }
        const prefixSize = offset;
        const multihashCode = next(); // multihash code
        const digestSize = next(); // multihash length
        const size = offset + digestSize;
        const multihashSize = size - prefixSize;
        return { version, codec, multihashCode, digestSize, multihashSize, size };
    }
    /**
     * Takes cid in a string representation and creates an instance. If `base`
     * decoder is not provided will use a default from the configuration. It will
     * throw an error if encoding of the CID is not compatible with supplied (or
     * a default decoder).
     */
    static parse(source, base) {
        const [prefix, bytes] = parseCIDtoBytes(source, base);
        const cid = CID.decode(bytes);
        if (cid.version === 0 && source[0] !== 'Q') {
            throw Error('Version 0 CID string must not include multibase prefix');
        }
        // Cache string representation to avoid computing it on `this.toString()`
        baseCache(cid).set(prefix, source);
        return cid;
    }
}
function parseCIDtoBytes(source, base) {
    switch (source[0]) {
        // CIDv0 is parsed differently
        case 'Q': {
            const decoder = base ?? base58btc;
            return [
                base58btc.prefix,
                decoder.decode(`${base58btc.prefix}${source}`)
            ];
        }
        case base58btc.prefix: {
            const decoder = base ?? base58btc;
            return [base58btc.prefix, decoder.decode(source)];
        }
        case base32.prefix: {
            const decoder = base ?? base32;
            return [base32.prefix, decoder.decode(source)];
        }
        case base36.prefix: {
            const decoder = base ?? base36;
            return [base36.prefix, decoder.decode(source)];
        }
        default: {
            if (base == null) {
                throw Error('To parse non base32, base36 or base58btc encoded CID multibase decoder must be provided');
            }
            return [source[0], base.decode(source)];
        }
    }
}
function toStringV0(bytes, cache, base) {
    const { prefix } = base;
    if (prefix !== base58btc.prefix) {
        throw Error(`Cannot string encode V0 in ${base.name} encoding`);
    }
    const cid = cache.get(prefix);
    if (cid == null) {
        const cid = base.encode(bytes).slice(1);
        cache.set(prefix, cid);
        return cid;
    }
    else {
        return cid;
    }
}
function toStringV1(bytes, cache, base) {
    const { prefix } = base;
    const cid = cache.get(prefix);
    if (cid == null) {
        const cid = base.encode(bytes);
        cache.set(prefix, cid);
        return cid;
    }
    else {
        return cid;
    }
}
const DAG_PB_CODE = 0x70;
const SHA_256_CODE = 0x12;
function encodeCID(version, code, multihash) {
    const codeOffset = encodingLength(version);
    const hashOffset = codeOffset + encodingLength(code);
    const bytes = new Uint8Array(hashOffset + multihash.byteLength);
    encodeTo(version, bytes, 0);
    encodeTo(code, bytes, codeOffset);
    bytes.set(multihash, hashOffset);
    return bytes;
}
const cidSymbol = Symbol.for('@ipld/js-cid/CID');
//# sourceMappingURL=cid.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/index.js
/**
 * @packageDocumentation
 *
 * This library defines common interfaces and low level building blocks for various interrelated multiformat technologies (multicodec, multihash, multibase, and CID). They can be used to implement custom base encoders / decoders / codecs, codec encoders /decoders and multihash hashers that comply to the interface that layers above assume.
 *
 * This library provides implementations for most basics and many others can be found in linked repositories.
 *
 * ```TypeScript
 * import { CID } from 'multiformats/cid'
 * import * as json from 'multiformats/codecs/json'
 * import { sha256 } from 'multiformats/hashes/sha2'
 *
 * const bytes = json.encode({ hello: 'world' })
 *
 * const hash = await sha256.digest(bytes)
 * const cid = CID.create(1, json.code, hash)
 * //> CID(bagaaierasords4njcts6vs7qvdjfcvgnume4hqohf65zsfguprqphs3icwea)
 * ```
 *
 * ## Creating Blocks
 *
 * ```TypeScript
 * import * as Block from 'multiformats/block'
 * import * as codec from '@ipld/dag-cbor'
 * import { sha256 as hasher } from 'multiformats/hashes/sha2'
 *
 * const value = { hello: 'world' }
 *
 * // encode a block
 * let block = await Block.encode({ value, codec, hasher })
 *
 * block.value // { hello: 'world' }
 * block.bytes // Uint8Array
 * block.cid   // CID() w/ sha2-256 hash address and dag-cbor codec
 *
 * // you can also decode blocks from their binary state
 * block = await Block.decode({ bytes: block.bytes, codec, hasher })
 *
 * // if you have the cid you can also verify the hash on decode
 * block = await Block.create({ bytes: block.bytes, cid: block.cid, codec, hasher })
 * ```
 *
 * ## Multibase Encoders / Decoders / Codecs
 *
 * CIDs can be serialized to string representation using multibase encoders that implement [`MultibaseEncoder`](https://github.com/multiformats/js-multiformats/blob/master/src/bases/interface.ts) interface. This library provides quite a few implementations that can be imported:
 *
 * ```TypeScript
 * import { base64 } from "multiformats/bases/base64"
 * cid.toString(base64.encoder)
 * //> 'mAYAEEiCTojlxqRTl6svwqNJRVM2jCcPBxy+7mRTUfGDzy2gViA'
 * ```
 *
 * Parsing CID string serialized CIDs requires multibase decoder that implements [`MultibaseDecoder`](https://github.com/multiformats/js-multiformats/blob/master/src/bases/interface.ts) interface. This library provides a decoder for every encoder it provides:
 *
 * ```TypeScript
 * CID.parse('mAYAEEiCTojlxqRTl6svwqNJRVM2jCcPBxy+7mRTUfGDzy2gViA', base64.decoder)
 * //> CID(bagaaierasords4njcts6vs7qvdjfcvgnume4hqohf65zsfguprqphs3icwea)
 * ```
 *
 * Dual of multibase encoder & decoder is defined as multibase codec and it exposes
 * them as `encoder` and `decoder` properties. For added convenience codecs also
 * implement `MultibaseEncoder` and `MultibaseDecoder` interfaces so they could be
 * used as either or both:
 *
 * ```TypeScript
 * cid.toString(base64)
 * CID.parse(cid.toString(base64), base64)
 * ```
 *
 * **Note:** CID implementation comes bundled with `base32` and `base58btc`
 * multibase codecs so that CIDs can be base serialized to (version specific)
 * default base encoding and parsed without having to supply base encoders/decoders:
 *
 * ```TypeScript
 * const v1 = CID.parse('bagaaierasords4njcts6vs7qvdjfcvgnume4hqohf65zsfguprqphs3icwea')
 * v1.toString()
 * //> 'bagaaierasords4njcts6vs7qvdjfcvgnume4hqohf65zsfguprqphs3icwea'
 *
 * const v0 = CID.parse('QmdfTbBqBPQ7VNxZEYEj14VmRuZBkqFbiwReogJgS1zR1n')
 * v0.toString()
 * //> 'QmdfTbBqBPQ7VNxZEYEj14VmRuZBkqFbiwReogJgS1zR1n'
 * v0.toV1().toString()
 * //> 'bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku'
 * ```
 *
 * ## Multicodec Encoders / Decoders / Codecs
 *
 * This library defines [`BlockEncoder`, `BlockDecoder` and `BlockCodec` interfaces](https://github.com/multiformats/js-multiformats/blob/master/src/codecs/interface.ts).
 * Codec implementations should conform to the `BlockCodec` interface which implements both `BlockEncoder` and `BlockDecoder`.
 * Here is an example implementation of JSON `BlockCodec`.
 *
 * ```TypeScript
 * export const { name, code, encode, decode } = {
 *   name: 'json',
 *   code: 0x0200,
 *   encode: json => new TextEncoder().encode(JSON.stringify(json)),
 *   decode: bytes => JSON.parse(new TextDecoder().decode(bytes))
 * }
 * ```
 *
 * ## Multihash Hashers
 *
 * This library defines [`MultihashHasher` and `MultihashDigest` interfaces](https://github.com/multiformats/js-multiformats/blob/master/src/hashes/interface.ts) and convinient function for implementing them:
 *
 * ```TypeScript
 * import * as hasher from 'multiformats/hashes/hasher'
 *
 * const sha256 = hasher.from({
 *   // As per multiformats table
 *   // https://github.com/multiformats/multicodec/blob/master/table.csv#L9
 *   name: 'sha2-256',
 *   code: 0x12,
 *
 *   encode: (input) => new Uint8Array(crypto.createHash('sha256').update(input).digest())
 * })
 *
 * const hash = await sha256.digest(json.encode({ hello: 'world' }))
 * CID.create(1, json.code, hash)
 *
 * //> CID(bagaaierasords4njcts6vs7qvdjfcvgnume4hqohf65zsfguprqphs3icwea)
 * ```
 *
 * ## Traversal
 *
 * This library contains higher-order functions for traversing graphs of data easily.
 *
 * `walk()` walks through the links in each block of a DAG calling a user-supplied loader function for each one, in depth-first order with no duplicate block visits. The loader should return a `Block` object and can be used to inspect and collect block ordering for a full DAG walk. The loader should `throw` on error, and return `null` if a block should be skipped by `walk()`.
 *
 * ```TypeScript
 * import { walk } from 'multiformats/traversal'
 * import * as Block from 'multiformats/block'
 * import * as codec from 'multiformats/codecs/json'
 * import { sha256 as hasher } from 'multiformats/hashes/sha2'
 *
 * // build a DAG (a single block for this simple example)
 * const value = { hello: 'world' }
 * const block = await Block.encode({ value, codec, hasher })
 * const { cid } = block
 * console.log(cid)
 * //> CID(bagaaierasords4njcts6vs7qvdjfcvgnume4hqohf65zsfguprqphs3icwea)
 *
 * // create a loader function that also collects CIDs of blocks in
 * // their traversal order
 * const load = (cid, blocks) => async (cid) => {
 *   // fetch a block using its cid
 *   // e.g.: const block = await fetchBlockByCID(cid)
 *   blocks.push(cid)
 *   return block
 * }
 *
 * // collect blocks in this DAG starting from the root `cid`
 * const blocks = []
 * await walk({ cid, load: load(cid, blocks) })
 *
 * console.log(blocks)
 * //> [CID(bagaaierasords4njcts6vs7qvdjfcvgnume4hqohf65zsfguprqphs3icwea)]
 * ```
 *
 * ## Legacy interface
 *
 * [`blockcodec-to-ipld-format`](https://github.com/ipld/js-blockcodec-to-ipld-format) converts a multiformats [`BlockCodec`](https://github.com/multiformats/js-multiformats/blob/master/src/codecs/interface.ts#L21) into an
 * [`interface-ipld-format`](https://github.com/ipld/interface-ipld-format) for use with the [`ipld`](https://github.com/ipld/ipld) package. This can help bridge IPLD codecs implemented using the structure and interfaces defined here to existing code that assumes, or requires `interface-ipld-format`. This bridge also includes the relevant TypeScript definitions.
 *
 * ## Implementations
 *
 * By default, no base encodings (other than base32 & base58btc), hash functions,
 * or codec implementations are exposed by `multiformats`, you need to
 * import the ones you need yourself.
 *
 * ### Multibase codecs
 *
 * | bases                                                         | import                      | repo                                                                                              |
 * | ------------------------------------------------------------- | --------------------------- | ------------------------------------------------------------------------------------------------- |
 * | `base16`                                                      | `multiformats/bases/base16` | [multiformats/js-multiformats](https://github.com/multiformats/js-multiformats/tree/master/bases) |
 * | `base32`, `base32pad`, `base32hex`, `base32hexpad`, `base32z` | `multiformats/bases/base32` | [multiformats/js-multiformats](https://github.com/multiformats/js-multiformats/tree/master/bases) |
 * | `base64`, `base64pad`, `base64url`, `base64urlpad`            | `multiformats/bases/base64` | [multiformats/js-multiformats](https://github.com/multiformats/js-multiformats/tree/master/bases) |
 * | `base58btc`, `base58flick4`                                   | `multiformats/bases/base58` | [multiformats/js-multiformats](https://github.com/multiformats/js-multiformats/tree/master/bases) |
 *
 * Other (less useful) bases implemented in [multiformats/js-multiformats](https://github.com/multiformats/js-multiformats/tree/master/bases) include: `base2`, `base8`, `base10`, `base36` and `base256emoji`.
 *
 * ### Multihash hashers
 *
 * | hashes                                                                                                                          | import                         | repo                                                                                                               |
 * | ------------------------------------------------------------------------------------------------------------------------------- | ------------------------------ | ------------------------------------------------------------------------------------------------------------------ |
 * | `sha2-256`, `sha2-512`                                                                                                          | `multiformats/hashes/sha2`     | [multiformats/js-multiformats](https://github.com/multiformats/js-multiformats/tree/master/src/hashes)             |
 * | `sha3-224`, `sha3-256`, `sha3-384`,`sha3-512`, `shake-128`, `shake-256`, `keccak-224`, `keccak-256`, `keccak-384`, `keccak-512` | `@multiformats/sha3`           | [multiformats/js-sha3](https://github.com/multiformats/js-sha3)                                                    |
 * | `identity`                                                                                                                      | `multiformats/hashes/identity` | [multiformats/js-multiformats](https://github.com/multiformats/js-multiformats/tree/master/src/hashes/identity.js) |
 * | `murmur3-128`, `murmur3-32`                                                                                                     | `@multiformats/murmur3`        | [multiformats/js-murmur3](https://github.com/multiformats/js-murmur3)                                              |
 * | `blake2b-*`, `blake2s-*`                                                                                                        | `@multiformats/blake2`         | [multiformats/js-blake2](https://github.com/multiformats/js-blake2)                                                |
 *
 * ### IPLD codecs (multicodec)
 *
 * | codec      | import                     | repo                                                                                                   |
 * | ---------- | -------------------------- | ------------------------------------------------------------------------------------------------------ |
 * | `raw`      | `multiformats/codecs/raw`  | [multiformats/js-multiformats](https://github.com/multiformats/js-multiformats/tree/master/src/codecs) |
 * | `json`     | `multiformats/codecs/json` | [multiformats/js-multiformats](https://github.com/multiformats/js-multiformats/tree/master/src/codecs) |
 * | `dag-cbor` | `@ipld/dag-cbor`           | [ipld/js-dag-cbor](https://github.com/ipld/js-dag-cbor)                                                |
 * | `dag-json` | `@ipld/dag-json`           | [ipld/js-dag-json](https://github.com/ipld/js-dag-json)                                                |
 * | `dag-pb`   | `@ipld/dag-pb`             | [ipld/js-dag-pb](https://github.com/ipld/js-dag-pb)                                                    |
 * | `dag-jose` | `dag-jose`                 | [ceramicnetwork/js-dag-jose](https://github.com/ceramicnetwork/js-dag-jose)                            |
 */





// This way TS will also expose all the types from module


//# sourceMappingURL=index.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/basics.js















const bases = { ...identity_namespaceObject, ...base2_namespaceObject, ...base8_namespaceObject, ...base10_namespaceObject, ...base16_namespaceObject, ...base32_namespaceObject, ...base36_namespaceObject, ...base58_namespaceObject, ...base64_namespaceObject, ...base256emoji_namespaceObject };
const hashes = { ...sha2_browser_namespaceObject, ...hashes_identity_namespaceObject };
const codecs = { raw: raw_namespaceObject, json: json_namespaceObject };

//# sourceMappingURL=basics.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/uint8arrays@5.1.0/node_modules/uint8arrays/dist/src/alloc.js
/**
 * Returns a `Uint8Array` of the requested size. Referenced memory will
 * be initialized to 0.
 */
function alloc(size = 0) {
    return new Uint8Array(size);
}
/**
 * Where possible returns a Uint8Array of the requested size that references
 * uninitialized memory. Only use if you are certain you will immediately
 * overwrite every value in the returned `Uint8Array`.
 */
function allocUnsafe(size = 0) {
    return new Uint8Array(size);
}
//# sourceMappingURL=alloc.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/uint8arrays@5.1.0/node_modules/uint8arrays/dist/src/util/bases.js


function createCodec(name, prefix, encode, decode) {
    return {
        name,
        prefix,
        encoder: {
            name,
            prefix,
            encode
        },
        decoder: {
            decode
        }
    };
}
const string = createCodec('utf8', 'u', (buf) => {
    const decoder = new TextDecoder('utf8');
    return 'u' + decoder.decode(buf);
}, (str) => {
    const encoder = new TextEncoder();
    return encoder.encode(str.substring(1));
});
const ascii = createCodec('ascii', 'a', (buf) => {
    let string = 'a';
    for (let i = 0; i < buf.length; i++) {
        string += String.fromCharCode(buf[i]);
    }
    return string;
}, (str) => {
    str = str.substring(1);
    const buf = allocUnsafe(str.length);
    for (let i = 0; i < str.length; i++) {
        buf[i] = str.charCodeAt(i);
    }
    return buf;
});
const BASES = {
    utf8: string,
    'utf-8': string,
    hex: bases.base16,
    latin1: ascii,
    ascii,
    binary: ascii,
    ...bases
};
/* harmony default export */ const util_bases = (BASES);
//# sourceMappingURL=bases.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/uint8arrays@5.1.0/node_modules/uint8arrays/dist/src/from-string.js

/**
 * Create a `Uint8Array` from the passed string
 *
 * Supports `utf8`, `utf-8`, `hex`, and any encoding supported by the multiformats module.
 *
 * Also `ascii` which is similar to node's 'binary' encoding.
 */
function from_string_fromString(string, encoding = 'utf8') {
    const base = util_bases[encoding];
    if (base == null) {
        throw new Error(`Unsupported encoding "${encoding}"`);
    }
    // add multibase prefix
    return base.decoder.decode(`${base.prefix}${string}`); // eslint-disable-line @typescript-eslint/restrict-template-expressions
}
//# sourceMappingURL=from-string.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/it-ws@6.1.5/node_modules/it-ws/dist/src/source.js


// copied from github.com/feross/buffer
// Some ArrayBuffers are not passing the instanceof check, so we need to do a bit more work :(
function isArrayBuffer(obj) {
    return (obj instanceof ArrayBuffer) ||
        (obj?.constructor?.name === 'ArrayBuffer' && typeof obj?.byteLength === 'number');
}
/* harmony default export */ const source = ((socket) => {
    socket.binaryType = 'arraybuffer';
    const connected = async () => {
        await new Promise((resolve, reject) => {
            if (isConnected) {
                resolve();
                return;
            }
            if (connError != null) {
                reject(connError);
                return;
            }
            const cleanUp = (cont) => {
                socket.removeEventListener('open', onOpen);
                socket.removeEventListener('error', onError);
                cont();
            };
            const onOpen = () => { cleanUp(resolve); };
            const onError = (event) => {
                cleanUp(() => { reject(event.error ?? new Error(`connect ECONNREFUSED ${socket.url}`)); });
            };
            socket.addEventListener('open', onOpen);
            socket.addEventListener('error', onError);
        });
    };
    const source = (async function* () {
        const messages = new dom/* EventIterator */.PP(({ push, stop, fail }) => {
            const onMessage = (event) => {
                let data = null;
                if (typeof event.data === 'string') {
                    data = from_string_fromString(event.data);
                }
                if (isArrayBuffer(event.data)) {
                    data = new Uint8Array(event.data);
                }
                if (event.data instanceof Uint8Array) {
                    data = event.data;
                }
                if (data == null) {
                    return;
                }
                push(data);
            };
            const onError = (event) => { fail(event.error ?? new Error('Socket error')); };
            socket.addEventListener('message', onMessage);
            socket.addEventListener('error', onError);
            socket.addEventListener('close', stop);
            return () => {
                socket.removeEventListener('message', onMessage);
                socket.removeEventListener('error', onError);
                socket.removeEventListener('close', stop);
            };
        }, { highWaterMark: Infinity });
        await connected();
        for await (const chunk of messages) {
            yield isArrayBuffer(chunk) ? new Uint8Array(chunk) : chunk;
        }
    }());
    let isConnected = socket.readyState === 1;
    let connError;
    socket.addEventListener('open', () => {
        isConnected = true;
        connError = null;
    });
    socket.addEventListener('close', () => {
        isConnected = false;
        connError = null;
    });
    socket.addEventListener('error', event => {
        if (!isConnected) {
            connError = event.error ?? new Error(`connect ECONNREFUSED ${socket.url}`);
        }
    });
    return Object.assign(source, {
        connected
    });
});
//# sourceMappingURL=source.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/it-ws@6.1.5/node_modules/it-ws/dist/src/duplex.js


/* harmony default export */ const duplex = ((socket, options) => {
    options = options ?? {};
    const connectedSource = source(socket);
    let remoteAddress = options.remoteAddress;
    let remotePort = options.remotePort;
    if (socket.url != null) {
        // only client->server sockets have urls, server->client connections do not
        try {
            const url = new URL(socket.url);
            remoteAddress = url.hostname;
            remotePort = parseInt(url.port, 10);
        }
        catch { }
    }
    if (remoteAddress == null || remotePort == null) {
        throw new Error('Remote connection did not have address and/or port');
    }
    const duplex = {
        sink: sink(socket, options),
        source: connectedSource,
        connected: async () => { await connectedSource.connected(); },
        close: async () => {
            if (socket.readyState === socket.CONNECTING || socket.readyState === socket.OPEN) {
                await new Promise((resolve) => {
                    socket.addEventListener('close', () => {
                        resolve();
                    });
                    socket.close();
                });
            }
        },
        destroy: () => {
            if (socket.terminate != null) {
                socket.terminate();
            }
            else {
                socket.close();
            }
        },
        remoteAddress,
        remotePort,
        socket
    };
    return duplex;
});
//# sourceMappingURL=duplex.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/it-ws@6.1.5/node_modules/it-ws/dist/src/web-socket.browser.js
/* eslint-env browser */
/* harmony default export */ const web_socket_browser = (WebSocket);
//# sourceMappingURL=web-socket.browser.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/it-ws@6.1.5/node_modules/it-ws/dist/src/ws-url.js
const map = { 'http:': 'ws:', 'https:': 'wss:' };
const defaultProtocol = 'ws:';
/* harmony default export */ const ws_url = ((url, location) => {
    if (url.startsWith('//')) {
        url = `${location?.protocol ?? defaultProtocol}${url}`;
    }
    if (url.startsWith('/') && location != null) {
        const proto = location.protocol ?? defaultProtocol;
        const host = location.host;
        const port = location.port != null && host?.endsWith(`:${location.port}`) !== true ? `:${location.port}` : '';
        url = `${proto}//${host}${port}${url}`;
    }
    const wsUrl = new URL(url);
    for (const [httpProto, wsProto] of Object.entries(map)) {
        if (wsUrl.protocol === httpProto) {
            wsUrl.protocol = wsProto;
        }
    }
    return wsUrl;
});
//# sourceMappingURL=ws-url.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/it-ws@6.1.5/node_modules/it-ws/dist/src/client.js
// load websocket library if we are not in the browser



function connect(addr, opts) {
    const location = typeof window === 'undefined' ? undefined : window.location;
    opts = opts ?? {};
    const url = ws_url(addr, location);
    // it's necessary to stringify the URL object otherwise react-native crashes
    const socket = new web_socket_browser(url.toString(), opts.websocket);
    return duplex(socket, opts);
}
//# sourceMappingURL=client.js.map

/***/ })

}]);
//# sourceMappingURL=5857.67f8b9bfaaa39ae3.js.map