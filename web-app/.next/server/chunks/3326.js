exports.id = 3326;
exports.ids = [3326];
exports.modules = {

/***/ 7026:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";
/* eslint no-unused-vars: ["error", { "varsIgnorePattern": "^Duplex" }] */ 
const { Duplex } = __webpack_require__(27910);
const { randomFillSync } = __webpack_require__(55511);
const PerMessageDeflate = __webpack_require__(33491);
const { EMPTY_BUFFER, kWebSocket, NOOP } = __webpack_require__(20670);
const { isBlob, isValidStatusCode } = __webpack_require__(82524);
const { mask: applyMask, toBuffer } = __webpack_require__(95954);
const kByteLength = Symbol('kByteLength');
const maskBuffer = Buffer.alloc(4);
const RANDOM_POOL_SIZE = 8 * 1024;
let randomPool;
let randomPoolPointer = RANDOM_POOL_SIZE;
const DEFAULT = 0;
const DEFLATING = 1;
const GET_BLOB_DATA = 2;
/**
 * HyBi Sender implementation.
 */ class Sender {
    /**
   * Creates a Sender instance.
   *
   * @param {Duplex} socket The connection socket
   * @param {Object} [extensions] An object containing the negotiated extensions
   * @param {Function} [generateMask] The function used to generate the masking
   *     key
   */ constructor(socket, extensions, generateMask){
        this._extensions = extensions || {};
        if (generateMask) {
            this._generateMask = generateMask;
            this._maskBuffer = Buffer.alloc(4);
        }
        this._socket = socket;
        this._firstFragment = true;
        this._compress = false;
        this._bufferedBytes = 0;
        this._queue = [];
        this._state = DEFAULT;
        this.onerror = NOOP;
        this[kWebSocket] = undefined;
    }
    /**
   * Frames a piece of data according to the HyBi WebSocket protocol.
   *
   * @param {(Buffer|String)} data The data to frame
   * @param {Object} options Options object
   * @param {Boolean} [options.fin=false] Specifies whether or not to set the
   *     FIN bit
   * @param {Function} [options.generateMask] The function used to generate the
   *     masking key
   * @param {Boolean} [options.mask=false] Specifies whether or not to mask
   *     `data`
   * @param {Buffer} [options.maskBuffer] The buffer used to store the masking
   *     key
   * @param {Number} options.opcode The opcode
   * @param {Boolean} [options.readOnly=false] Specifies whether `data` can be
   *     modified
   * @param {Boolean} [options.rsv1=false] Specifies whether or not to set the
   *     RSV1 bit
   * @return {(Buffer|String)[]} The framed data
   * @public
   */ static frame(data, options) {
        let mask;
        let merge = false;
        let offset = 2;
        let skipMasking = false;
        if (options.mask) {
            mask = options.maskBuffer || maskBuffer;
            if (options.generateMask) {
                options.generateMask(mask);
            } else {
                if (randomPoolPointer === RANDOM_POOL_SIZE) {
                    /* istanbul ignore else  */ if (randomPool === undefined) {
                        //
                        // This is lazily initialized because server-sent frames must not
                        // be masked so it may never be used.
                        //
                        randomPool = Buffer.alloc(RANDOM_POOL_SIZE);
                    }
                    randomFillSync(randomPool, 0, RANDOM_POOL_SIZE);
                    randomPoolPointer = 0;
                }
                mask[0] = randomPool[randomPoolPointer++];
                mask[1] = randomPool[randomPoolPointer++];
                mask[2] = randomPool[randomPoolPointer++];
                mask[3] = randomPool[randomPoolPointer++];
            }
            skipMasking = (mask[0] | mask[1] | mask[2] | mask[3]) === 0;
            offset = 6;
        }
        let dataLength;
        if (typeof data === 'string') {
            if ((!options.mask || skipMasking) && options[kByteLength] !== undefined) {
                dataLength = options[kByteLength];
            } else {
                data = Buffer.from(data);
                dataLength = data.length;
            }
        } else {
            dataLength = data.length;
            merge = options.mask && options.readOnly && !skipMasking;
        }
        let payloadLength = dataLength;
        if (dataLength >= 65536) {
            offset += 8;
            payloadLength = 127;
        } else if (dataLength > 125) {
            offset += 2;
            payloadLength = 126;
        }
        const target = Buffer.allocUnsafe(merge ? dataLength + offset : offset);
        target[0] = options.fin ? options.opcode | 0x80 : options.opcode;
        if (options.rsv1) target[0] |= 0x40;
        target[1] = payloadLength;
        if (payloadLength === 126) {
            target.writeUInt16BE(dataLength, 2);
        } else if (payloadLength === 127) {
            target[2] = target[3] = 0;
            target.writeUIntBE(dataLength, 4, 6);
        }
        if (!options.mask) return [
            target,
            data
        ];
        target[1] |= 0x80;
        target[offset - 4] = mask[0];
        target[offset - 3] = mask[1];
        target[offset - 2] = mask[2];
        target[offset - 1] = mask[3];
        if (skipMasking) return [
            target,
            data
        ];
        if (merge) {
            applyMask(data, mask, target, offset, dataLength);
            return [
                target
            ];
        }
        applyMask(data, mask, data, 0, dataLength);
        return [
            target,
            data
        ];
    }
    /**
   * Sends a close message to the other peer.
   *
   * @param {Number} [code] The status code component of the body
   * @param {(String|Buffer)} [data] The message component of the body
   * @param {Boolean} [mask=false] Specifies whether or not to mask the message
   * @param {Function} [cb] Callback
   * @public
   */ close(code, data, mask, cb) {
        let buf;
        if (code === undefined) {
            buf = EMPTY_BUFFER;
        } else if (typeof code !== 'number' || !isValidStatusCode(code)) {
            throw new TypeError('First argument must be a valid error code number');
        } else if (data === undefined || !data.length) {
            buf = Buffer.allocUnsafe(2);
            buf.writeUInt16BE(code, 0);
        } else {
            const length = Buffer.byteLength(data);
            if (length > 123) {
                throw new RangeError('The message must not be greater than 123 bytes');
            }
            buf = Buffer.allocUnsafe(2 + length);
            buf.writeUInt16BE(code, 0);
            if (typeof data === 'string') {
                buf.write(data, 2);
            } else {
                buf.set(data, 2);
            }
        }
        const options = {
            [kByteLength]: buf.length,
            fin: true,
            generateMask: this._generateMask,
            mask,
            maskBuffer: this._maskBuffer,
            opcode: 0x08,
            readOnly: false,
            rsv1: false
        };
        if (this._state !== DEFAULT) {
            this.enqueue([
                this.dispatch,
                buf,
                false,
                options,
                cb
            ]);
        } else {
            this.sendFrame(Sender.frame(buf, options), cb);
        }
    }
    /**
   * Sends a ping message to the other peer.
   *
   * @param {*} data The message to send
   * @param {Boolean} [mask=false] Specifies whether or not to mask `data`
   * @param {Function} [cb] Callback
   * @public
   */ ping(data, mask, cb) {
        let byteLength;
        let readOnly;
        if (typeof data === 'string') {
            byteLength = Buffer.byteLength(data);
            readOnly = false;
        } else if (isBlob(data)) {
            byteLength = data.size;
            readOnly = false;
        } else {
            data = toBuffer(data);
            byteLength = data.length;
            readOnly = toBuffer.readOnly;
        }
        if (byteLength > 125) {
            throw new RangeError('The data size must not be greater than 125 bytes');
        }
        const options = {
            [kByteLength]: byteLength,
            fin: true,
            generateMask: this._generateMask,
            mask,
            maskBuffer: this._maskBuffer,
            opcode: 0x09,
            readOnly,
            rsv1: false
        };
        if (isBlob(data)) {
            if (this._state !== DEFAULT) {
                this.enqueue([
                    this.getBlobData,
                    data,
                    false,
                    options,
                    cb
                ]);
            } else {
                this.getBlobData(data, false, options, cb);
            }
        } else if (this._state !== DEFAULT) {
            this.enqueue([
                this.dispatch,
                data,
                false,
                options,
                cb
            ]);
        } else {
            this.sendFrame(Sender.frame(data, options), cb);
        }
    }
    /**
   * Sends a pong message to the other peer.
   *
   * @param {*} data The message to send
   * @param {Boolean} [mask=false] Specifies whether or not to mask `data`
   * @param {Function} [cb] Callback
   * @public
   */ pong(data, mask, cb) {
        let byteLength;
        let readOnly;
        if (typeof data === 'string') {
            byteLength = Buffer.byteLength(data);
            readOnly = false;
        } else if (isBlob(data)) {
            byteLength = data.size;
            readOnly = false;
        } else {
            data = toBuffer(data);
            byteLength = data.length;
            readOnly = toBuffer.readOnly;
        }
        if (byteLength > 125) {
            throw new RangeError('The data size must not be greater than 125 bytes');
        }
        const options = {
            [kByteLength]: byteLength,
            fin: true,
            generateMask: this._generateMask,
            mask,
            maskBuffer: this._maskBuffer,
            opcode: 0x0a,
            readOnly,
            rsv1: false
        };
        if (isBlob(data)) {
            if (this._state !== DEFAULT) {
                this.enqueue([
                    this.getBlobData,
                    data,
                    false,
                    options,
                    cb
                ]);
            } else {
                this.getBlobData(data, false, options, cb);
            }
        } else if (this._state !== DEFAULT) {
            this.enqueue([
                this.dispatch,
                data,
                false,
                options,
                cb
            ]);
        } else {
            this.sendFrame(Sender.frame(data, options), cb);
        }
    }
    /**
   * Sends a data message to the other peer.
   *
   * @param {*} data The message to send
   * @param {Object} options Options object
   * @param {Boolean} [options.binary=false] Specifies whether `data` is binary
   *     or text
   * @param {Boolean} [options.compress=false] Specifies whether or not to
   *     compress `data`
   * @param {Boolean} [options.fin=false] Specifies whether the fragment is the
   *     last one
   * @param {Boolean} [options.mask=false] Specifies whether or not to mask
   *     `data`
   * @param {Function} [cb] Callback
   * @public
   */ send(data, options, cb) {
        const perMessageDeflate = this._extensions[PerMessageDeflate.extensionName];
        let opcode = options.binary ? 2 : 1;
        let rsv1 = options.compress;
        let byteLength;
        let readOnly;
        if (typeof data === 'string') {
            byteLength = Buffer.byteLength(data);
            readOnly = false;
        } else if (isBlob(data)) {
            byteLength = data.size;
            readOnly = false;
        } else {
            data = toBuffer(data);
            byteLength = data.length;
            readOnly = toBuffer.readOnly;
        }
        if (this._firstFragment) {
            this._firstFragment = false;
            if (rsv1 && perMessageDeflate && perMessageDeflate.params[perMessageDeflate._isServer ? 'server_no_context_takeover' : 'client_no_context_takeover']) {
                rsv1 = byteLength >= perMessageDeflate._threshold;
            }
            this._compress = rsv1;
        } else {
            rsv1 = false;
            opcode = 0;
        }
        if (options.fin) this._firstFragment = true;
        const opts = {
            [kByteLength]: byteLength,
            fin: options.fin,
            generateMask: this._generateMask,
            mask: options.mask,
            maskBuffer: this._maskBuffer,
            opcode,
            readOnly,
            rsv1
        };
        if (isBlob(data)) {
            if (this._state !== DEFAULT) {
                this.enqueue([
                    this.getBlobData,
                    data,
                    this._compress,
                    opts,
                    cb
                ]);
            } else {
                this.getBlobData(data, this._compress, opts, cb);
            }
        } else if (this._state !== DEFAULT) {
            this.enqueue([
                this.dispatch,
                data,
                this._compress,
                opts,
                cb
            ]);
        } else {
            this.dispatch(data, this._compress, opts, cb);
        }
    }
    /**
   * Gets the contents of a blob as binary data.
   *
   * @param {Blob} blob The blob
   * @param {Boolean} [compress=false] Specifies whether or not to compress
   *     the data
   * @param {Object} options Options object
   * @param {Boolean} [options.fin=false] Specifies whether or not to set the
   *     FIN bit
   * @param {Function} [options.generateMask] The function used to generate the
   *     masking key
   * @param {Boolean} [options.mask=false] Specifies whether or not to mask
   *     `data`
   * @param {Buffer} [options.maskBuffer] The buffer used to store the masking
   *     key
   * @param {Number} options.opcode The opcode
   * @param {Boolean} [options.readOnly=false] Specifies whether `data` can be
   *     modified
   * @param {Boolean} [options.rsv1=false] Specifies whether or not to set the
   *     RSV1 bit
   * @param {Function} [cb] Callback
   * @private
   */ getBlobData(blob, compress, options, cb) {
        this._bufferedBytes += options[kByteLength];
        this._state = GET_BLOB_DATA;
        blob.arrayBuffer().then((arrayBuffer)=>{
            if (this._socket.destroyed) {
                const err = new Error('The socket was closed while the blob was being read');
                //
                // `callCallbacks` is called in the next tick to ensure that errors
                // that might be thrown in the callbacks behave like errors thrown
                // outside the promise chain.
                //
                process.nextTick(callCallbacks, this, err, cb);
                return;
            }
            this._bufferedBytes -= options[kByteLength];
            const data = toBuffer(arrayBuffer);
            if (!compress) {
                this._state = DEFAULT;
                this.sendFrame(Sender.frame(data, options), cb);
                this.dequeue();
            } else {
                this.dispatch(data, compress, options, cb);
            }
        }).catch((err)=>{
            //
            // `onError` is called in the next tick for the same reason that
            // `callCallbacks` above is.
            //
            process.nextTick(onError, this, err, cb);
        });
    }
    /**
   * Dispatches a message.
   *
   * @param {(Buffer|String)} data The message to send
   * @param {Boolean} [compress=false] Specifies whether or not to compress
   *     `data`
   * @param {Object} options Options object
   * @param {Boolean} [options.fin=false] Specifies whether or not to set the
   *     FIN bit
   * @param {Function} [options.generateMask] The function used to generate the
   *     masking key
   * @param {Boolean} [options.mask=false] Specifies whether or not to mask
   *     `data`
   * @param {Buffer} [options.maskBuffer] The buffer used to store the masking
   *     key
   * @param {Number} options.opcode The opcode
   * @param {Boolean} [options.readOnly=false] Specifies whether `data` can be
   *     modified
   * @param {Boolean} [options.rsv1=false] Specifies whether or not to set the
   *     RSV1 bit
   * @param {Function} [cb] Callback
   * @private
   */ dispatch(data, compress, options, cb) {
        if (!compress) {
            this.sendFrame(Sender.frame(data, options), cb);
            return;
        }
        const perMessageDeflate = this._extensions[PerMessageDeflate.extensionName];
        this._bufferedBytes += options[kByteLength];
        this._state = DEFLATING;
        perMessageDeflate.compress(data, options.fin, (_, buf)=>{
            if (this._socket.destroyed) {
                const err = new Error('The socket was closed while data was being compressed');
                callCallbacks(this, err, cb);
                return;
            }
            this._bufferedBytes -= options[kByteLength];
            this._state = DEFAULT;
            options.readOnly = false;
            this.sendFrame(Sender.frame(buf, options), cb);
            this.dequeue();
        });
    }
    /**
   * Executes queued send operations.
   *
   * @private
   */ dequeue() {
        while(this._state === DEFAULT && this._queue.length){
            const params = this._queue.shift();
            this._bufferedBytes -= params[3][kByteLength];
            Reflect.apply(params[0], this, params.slice(1));
        }
    }
    /**
   * Enqueues a send operation.
   *
   * @param {Array} params Send operation parameters.
   * @private
   */ enqueue(params) {
        this._bufferedBytes += params[3][kByteLength];
        this._queue.push(params);
    }
    /**
   * Sends a frame.
   *
   * @param {(Buffer | String)[]} list The frame to send
   * @param {Function} [cb] Callback
   * @private
   */ sendFrame(list, cb) {
        if (list.length === 2) {
            this._socket.cork();
            this._socket.write(list[0]);
            this._socket.write(list[1], cb);
            this._socket.uncork();
        } else {
            this._socket.write(list[0], cb);
        }
    }
}
module.exports = Sender;
/**
 * Calls queued callbacks with an error.
 *
 * @param {Sender} sender The `Sender` instance
 * @param {Error} err The error to call the callbacks with
 * @param {Function} [cb] The first callback
 * @private
 */ function callCallbacks(sender, err, cb) {
    if (typeof cb === 'function') cb(err);
    for(let i = 0; i < sender._queue.length; i++){
        const params = sender._queue[i];
        const callback = params[params.length - 1];
        if (typeof callback === 'function') callback(err);
    }
}
/**
 * Handles a `Sender` error.
 *
 * @param {Sender} sender The `Sender` instance
 * @param {Error} err The error
 * @param {Function} [cb] The first pending callback
 * @private
 */ function onError(sender, err, cb) {
    callCallbacks(sender, err, cb);
    sender.onerror(err);
}


/***/ }),

/***/ 12873:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";


const { tokenChars } = __webpack_require__(82524);

/**
 * Parses the `Sec-WebSocket-Protocol` header into a set of subprotocol names.
 *
 * @param {String} header The field value of the header
 * @return {Set} The subprotocol names
 * @public
 */
function parse(header) {
  const protocols = new Set();
  let start = -1;
  let end = -1;
  let i = 0;

  for (i; i < header.length; i++) {
    const code = header.charCodeAt(i);

    if (end === -1 && tokenChars[code] === 1) {
      if (start === -1) start = i;
    } else if (
      i !== 0 &&
      (code === 0x20 /* ' ' */ || code === 0x09) /* '\t' */
    ) {
      if (end === -1 && start !== -1) end = i;
    } else if (code === 0x2c /* ',' */) {
      if (start === -1) {
        throw new SyntaxError(`Unexpected character at index ${i}`);
      }

      if (end === -1) end = i;

      const protocol = header.slice(start, end);

      if (protocols.has(protocol)) {
        throw new SyntaxError(`The "${protocol}" subprotocol is duplicated`);
      }

      protocols.add(protocol);
      start = end = -1;
    } else {
      throw new SyntaxError(`Unexpected character at index ${i}`);
    }
  }

  if (start === -1 || end !== -1) {
    throw new SyntaxError('Unexpected end of input');
  }

  const protocol = header.slice(start, i);

  if (protocols.has(protocol)) {
    throw new SyntaxError(`The "${protocol}" subprotocol is duplicated`);
  }

  protocols.add(protocol);
  return protocols;
}

module.exports = { parse };


/***/ }),

/***/ 13326:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
// ESM COMPAT FLAG
__webpack_require__.r(__webpack_exports__);

// EXPORTS
__webpack_require__.d(__webpack_exports__, {
  TerminalOutput: () => (/* binding */ TerminalOutput)
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

// NAMESPACE OBJECT: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/hashes/sha2.js
var sha2_namespaceObject = {};
__webpack_require__.r(sha2_namespaceObject);
__webpack_require__.d(sha2_namespaceObject, {
  sha256: () => (sha256),
  sha512: () => (sha512)
});

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-jsx-runtime.js
var react_jsx_runtime = __webpack_require__(79147);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react.js
var react = __webpack_require__(88782);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/next@15.3.2+c47b41e60e61c3b4/node_modules/next/dist/server/route-modules/app-page/vendored/ssr/react-dom.js
var react_dom = __webpack_require__(68051);
;// ./src/lib/terminal/modifierKeys.ts
// xterm modifier key sequences (CSI parameter convention: 2=Shift, 3=Alt, 5=Ctrl).
// Keyed by the unmodified sequence the toolbar sends; callers fall back to the
// input (or an ESC prefix, for Alt) when a key has no entry.
const CTRL_KEY_MAP = {
    '\x1b[A': '\x1b[1;5A',
    '\x1b[B': '\x1b[1;5B',
    '\x1b[C': '\x1b[1;5C',
    '\x1b[D': '\x1b[1;5D',
    '\x1b[H': '\x1b[1;5H',
    '\x1b[F': '\x1b[1;5F',
    '\x1b[5~': '\x1b[5;5~',
    '\x1b[6~': '\x1b[6;5~',
    '/': '\x1f',
    '-': '\x1f'
};
const ALT_KEY_MAP = {
    '\x1b[A': '\x1b[1;3A',
    '\x1b[B': '\x1b[1;3B',
    '\x1b[C': '\x1b[1;3C',
    '\x1b[D': '\x1b[1;3D',
    '\x1b[H': '\x1b[1;3H',
    '\x1b[F': '\x1b[1;3F',
    '\x1b[5~': '\x1b[5;3~',
    '\x1b[6~': '\x1b[6;3~'
};
const SHIFT_KEY_MAP = {
    '\t': '\x1b[Z',
    '\x1b[A': '\x1b[1;2A',
    '\x1b[B': '\x1b[1;2B',
    '\x1b[C': '\x1b[1;2C',
    '\x1b[D': '\x1b[1;2D',
    '\x1b[H': '\x1b[1;2H',
    '\x1b[F': '\x1b[1;2F',
    '\x1b[5~': '\x1b[5;2~',
    '\x1b[6~': '\x1b[6;2~'
};

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/promise-client.js + 2 modules
var promise_client = __webpack_require__(4435);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/connect-error.js
var connect_error = __webpack_require__(22800);
// EXTERNAL MODULE: ./src/gen/session/v1/session_pb.ts
var session_pb = __webpack_require__(31381);
// EXTERNAL MODULE: ./src/gen/session/v1/events_pb.ts
var events_pb = __webpack_require__(15588);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@bufbuild+protobuf@2.11.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
var create = __webpack_require__(7094);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/context-values.js
var context_values = __webpack_require__(6797);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/code.js
var esm_code = __webpack_require__(70792);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/serialization.js + 1 modules
var serialization = __webpack_require__(96548);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/run-call.js + 3 modules
var run_call = __webpack_require__(72337);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/create-method-url.js
var create_method_url = __webpack_require__(50452);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol/compression.js
var compression = __webpack_require__(40720);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol-connect/request-header.js
var request_header = __webpack_require__(75832);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect@2.1.1+d35040c863cffc59/node_modules/@connectrpc/connect/dist/esm/protocol-connect/end-stream.js
var end_stream = __webpack_require__(27125);
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
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/event-iterator@2.0.0/node_modules/event-iterator/lib/node.js
var node = __webpack_require__(38519);
// EXTERNAL MODULE: external "node:buffer"
var external_node_buffer_ = __webpack_require__(4573);
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
function digest_create(code, digest) {
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
    return digest_create(identity_code, identity_encode(input));
}
const identity_identity = { code: identity_code, name: identity_name, encode: identity_encode, digest };
//# sourceMappingURL=identity.js.map
// EXTERNAL MODULE: external "crypto"
var external_crypto_ = __webpack_require__(55511);
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
    return digest_create(code, digest);
}
//# sourceMappingURL=hasher.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/multiformats@13.4.1/node_modules/multiformats/dist/src/hashes/sha2.js



const sha256 = hasher_from({
    name: 'sha2-256',
    code: 0x12,
    encode: (input) => coerce(external_crypto_.createHash('sha256').update(input).digest())
});
const sha512 = hasher_from({
    name: 'sha2-512',
    code: 0x13,
    encode: input => coerce(external_crypto_.createHash('sha512').update(input).digest())
});
//# sourceMappingURL=sha2.js.map
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
                const multihash = digest_create(code, digest);
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
const hashes = { ...sha2_namespaceObject, ...hashes_identity_namespaceObject };
const codecs = { raw: raw_namespaceObject, json: json_namespaceObject };

//# sourceMappingURL=basics.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/uint8arrays@5.1.0/node_modules/uint8arrays/dist/src/util/as-uint8array.node.js
/**
 * To guarantee Uint8Array semantics, convert nodejs Buffers
 * into vanilla Uint8Arrays
 */
function as_uint8array_node_asUint8Array(buf) {
    return new Uint8Array(buf.buffer, buf.byteOffset, buf.byteLength);
}
//# sourceMappingURL=as-uint8array.node.js.map
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/uint8arrays@5.1.0/node_modules/uint8arrays/dist/src/alloc.node.js


/**
 * Returns a `Uint8Array` of the requested size. Referenced memory will
 * be initialized to 0.
 */
function alloc(size = 0) {
    return asUint8Array(Buffer.alloc(size));
}
/**
 * Where possible returns a Uint8Array of the requested size that references
 * uninitialized memory. Only use if you are certain you will immediately
 * overwrite every value in the returned `Uint8Array`.
 */
function allocUnsafe(size = 0) {
    return as_uint8array_node_asUint8Array(external_node_buffer_.Buffer.allocUnsafe(size));
}
//# sourceMappingURL=alloc.node.js.map
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
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/uint8arrays@5.1.0/node_modules/uint8arrays/dist/src/from-string.node.js



/**
 * Create a `Uint8Array` from the passed string
 *
 * Supports `utf8`, `utf-8`, `hex`, and any encoding supported by the multiformats module.
 *
 * Also `ascii` which is similar to node's 'binary' encoding.
 */
function from_string_node_fromString(string, encoding = 'utf8') {
    const base = util_bases[encoding];
    if (base == null) {
        throw new Error(`Unsupported encoding "${encoding}"`);
    }
    if (encoding === 'utf8' || encoding === 'utf-8') {
        return as_uint8array_node_asUint8Array(external_node_buffer_.Buffer.from(string, 'utf-8'));
    }
    // add multibase prefix
    return base.decoder.decode(`${base.prefix}${string}`); // eslint-disable-line @typescript-eslint/restrict-template-expressions
}
//# sourceMappingURL=from-string.node.js.map
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
        const messages = new node/* EventIterator */.PP(({ push, stop, fail }) => {
            const onMessage = (event) => {
                let data = null;
                if (typeof event.data === 'string') {
                    data = from_string_node_fromString(event.data);
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
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/ws@8.18.3/node_modules/ws/lib/stream.js
var stream = __webpack_require__(70623);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/ws@8.18.3/node_modules/ws/lib/receiver.js
var receiver = __webpack_require__(69062);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/ws@8.18.3/node_modules/ws/lib/sender.js
var sender = __webpack_require__(7026);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/ws@8.18.3/node_modules/ws/lib/websocket.js
var websocket = __webpack_require__(55696);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/ws@8.18.3/node_modules/ws/lib/websocket-server.js
var websocket_server = __webpack_require__(68226);
;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/ws@8.18.3/node_modules/ws/wrapper.mjs







/* harmony default export */ const wrapper = (websocket);

;// ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/it-ws@6.1.5/node_modules/it-ws/dist/src/web-socket.js

/* harmony default export */ const web_socket = (wrapper);
//# sourceMappingURL=web-socket.js.map
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
    const socket = new web_socket(url.toString(), opts.websocket);
    return duplex(socket, opts);
}
//# sourceMappingURL=client.js.map
;// ./src/lib/transport/websocket-transport.ts
/* __next_internal_client_entry_do_not_use__ decompressGzipPayload,createWebsocketBasedTransport auto */ 



/**
 * Check if debug logging is enabled via localStorage
 */ function isDebugEnabled() {
    if (true) return false;
    return localStorage.getItem("debug-terminal") === "true";
}
/**
 * Encodes a message in ConnectRPC envelope format.
 * Format: [1 byte flags][4 bytes big-endian length][N bytes data]
 */ function encodeEnvelope(flags, data) {
    const envelope = new Uint8Array(5 + data.length);
    envelope[0] = flags;
    // Write length as big-endian uint32
    const view = new DataView(envelope.buffer, envelope.byteOffset, envelope.byteLength);
    view.setUint32(1, data.length, false); // false = big-endian
    envelope.set(data, 5);
    return envelope;
}
/**
 * Decompresses a gzip-compressed envelope payload (terminal-resync-reliability Epic 5.1,
 * Task 5.1.1.0/5.1.1.2). The server sets the envelope's CompressedFlag (see
 * server/protocol/envelope.go) when a payload exceeds its size threshold; this mirrors that
 * on the client so parseResponseBody can decode the frame instead of hard-throwing.
 *
 * Built on ReadableStream + DecompressionStream directly (rather than Response/Blob.stream())
 * so it also runs under Jest's jsdom environment, which polyfills those two from
 * node:stream/web (see jest.setup.js) but does not provide a global Response.
 */ async function decompressGzipPayload(data) {
    // Typed as Uint8Array<ArrayBuffer> (rather than the bare Uint8Array default of
    // Uint8Array<ArrayBufferLike>) to match DecompressionStream.readable's declared type in
    // lib.dom.d.ts exactly — otherwise pipeThrough's generic inference can't reconcile the two
    // and tsc rejects the call.
    const stream = new ReadableStream({
        start (controller) {
            controller.enqueue(data);
            controller.close();
        }
    });
    const decompressed = stream.pipeThrough(new DecompressionStream("gzip"));
    const reader = decompressed.getReader();
    const chunks = [];
    let totalLength = 0;
    for(;;){
        const { done, value } = await reader.read();
        if (done) break;
        chunks.push(value);
        totalLength += value.length;
    }
    const result = new Uint8Array(totalLength);
    let offset = 0;
    for (const chunk of chunks){
        result.set(chunk, offset);
        offset += chunk.length;
    }
    return result;
}
/**
 * Creates a ConnectRPC transport that uses WebSocket for streaming calls.
 * This works around browser fetch() API limitations for bidirectional streaming.
 *
 * Based on: https://gist.github.com/Cyberax/3956c935a7971627e2ce8e2df3fafb8e
 */ function createWebsocketBasedTransport(opt) {
    return {
        // Use standard fetch for unary calls
        async unary (method, signal, timeoutMs, header, input, contextValues) {
            const useBinaryFormat = opt.useBinaryFormat ?? true;
            const { serialize, parse } = (0,serialization/* createClientMethodSerializers */.Ef)(method, useBinaryFormat, opt.jsonOptions, opt.binaryOptions);
            return await (0,run_call/* runUnaryCall */.L)({
                signal,
                interceptors: opt.interceptors,
                req: {
                    stream: false,
                    service: method.parent,
                    method,
                    url: (0,create_method_url/* createMethodUrl */.j)(opt.baseUrl, method),
                    requestMethod: "POST",
                    header: (0,request_header/* requestHeader */.L)(method.methodKind, useBinaryFormat, timeoutMs, header, false),
                    contextValues: contextValues ?? (0,context_values/* createContextValues */.k)(),
                    message: input
                },
                next: async (req)=>{
                    const response = await fetch(req.url, {
                        method: "POST",
                        mode: "cors",
                        headers: req.header,
                        body: serialize(req.message),
                        signal: req.signal
                    });
                    // Check response status
                    if (!response.ok) {
                        const code = response.status === 401 ? esm_code/* Code */.C.Unauthenticated : esm_code/* Code */.C.Unknown;
                        throw new connect_error/* ConnectError */.T(`HTTP ${response.status}: ${response.statusText}`, code);
                    }
                    const bodyBytes = new Uint8Array(await response.arrayBuffer());
                    const trailer = new Headers();
                    const message = parse(bodyBytes);
                    return {
                        stream: false,
                        header: response.headers,
                        message,
                        trailer,
                        service: method.parent,
                        method
                    };
                }
            });
        },
        // Use WebSocket for streaming calls
        async stream (method, signal, timeoutMs, header, input, contextValues) {
            const useBinaryFormat = opt.useBinaryFormat ?? true;
            const { serialize, parse } = (0,serialization/* createClientMethodSerializers */.Ef)(method, useBinaryFormat, opt.jsonOptions, opt.binaryOptions);
            // Parse envelope protocol messages from WebSocket stream
            async function* parseResponseBody(body, trailerTarget, headerRef, getCloseCode) {
                const reader = createEnvelopeReadableStreamForWS(body).getReader();
                let endStreamReceived = false;
                for(;;){
                    const result = await reader.read();
                    if (result.done) {
                        break;
                    }
                    const { flags, data } = result.value;
                    // The end-stream frame carries JSON trailer/error metadata (see endStreamFromJson
                    // below) and is never compressed by the server, so it must be checked ahead of the
                    // compressed-flag branch.
                    if ((flags & end_stream/* endStreamFlag */.Cu) === end_stream/* endStreamFlag */.Cu) {
                        endStreamReceived = true;
                        const endStream = (0,end_stream/* endStreamFromJson */.mo)(data);
                        if (endStream.error) {
                            const error = endStream.error;
                            headerRef.forEach((value, key)=>{
                                error.metadata.append(key, value);
                            });
                            throw error;
                        }
                        endStream.metadata.forEach((value, key)=>trailerTarget.set(key, value));
                        continue;
                    }
                    let payload = data;
                    if ((flags & compression/* compressedFlag */.y) === compression/* compressedFlag */.y) {
                        try {
                            payload = await decompressGzipPayload(data);
                        } catch (err) {
                            throw new connect_error/* ConnectError */.T(`protocol error: failed to decompress gzip payload: ${err instanceof Error ? err.message : String(err)}`, esm_code/* Code */.C.DataLoss);
                        }
                    }
                    yield parse(payload);
                }
                if (!endStreamReceived) {
                    const code = getCloseCode();
                    if (code !== null && code !== 1000 && !signal?.aborted) {
                        throw new connect_error/* ConnectError */.T("WebSocket closed", esm_code/* Code */.C.Unavailable, new Headers({
                            "ws-close-code": String(code)
                        }));
                    }
                    throw new connect_error/* ConnectError */.T("stream ended without end-stream message", esm_code/* Code */.C.Internal);
                }
            }
            // Create request body for streaming call
            // For server streaming: send single request message
            // For bidirectional streaming: send initial request, then stream via WebSocket
            async function createRequestBody(input) {
                const r = await input[Symbol.asyncIterator]().next();
                if (r.done === true) {
                    throw new Error("missing request message");
                }
                return encodeEnvelope(0, serialize(r.value));
            }
            timeoutMs = timeoutMs === undefined ? opt.defaultTimeoutMs : timeoutMs <= 0 ? undefined : timeoutMs;
            return await (0,run_call/* runStreamingCall */.u)({
                interceptors: opt.interceptors,
                timeoutMs,
                signal,
                req: {
                    stream: true,
                    service: method.parent,
                    method,
                    url: (0,create_method_url/* createMethodUrl */.j)(opt.baseUrl, method),
                    requestMethod: "POST",
                    header: (0,request_header/* requestHeader */.L)(method.methodKind, useBinaryFormat, timeoutMs, header, false),
                    contextValues: contextValues ?? (0,context_values/* createContextValues */.k)(),
                    message: input
                },
                next: async (req)=>{
                    // Convert HTTP URL to WebSocket URL
                    const wsUrl = req.url.replace(/^http/, "ws");
                    // Connect to WebSocket using it-ws
                    const stream = connect(wsUrl);
                    // Capture the WebSocket close code so we can propagate it to the hook
                    // via a ConnectError metadata field. The it-ws source generator calls
                    // EventIterator `stop` on close (which ends the async generator without
                    // an error), so we capture it externally here.
                    let wsCloseCode = null;
                    stream.socket.addEventListener("close", (ev)=>{
                        wsCloseCode = ev.code;
                    });
                    if (signal !== undefined) {
                        if (signal.aborted) stream.destroy();
                        else signal.addEventListener("abort", ()=>stream.destroy(), {
                            once: true
                        });
                    }
                    // Wait for connection
                    await stream.connected();
                    // Send headers as text message
                    let headerText = "";
                    req.header.forEach((value, key)=>{
                        headerText += `${key}: ${value}\r\n`;
                    });
                    stream.socket.send(headerText + "\r\n");
                    // Send all messages from input iterator (including initial handshake)
                    (async ()=>{
                        try {
                            for await (const msg of req.message){
                                const serialized = serialize(msg);
                                if (isDebugEnabled()) {
                                    console.log(`[WebSocket] Sending message:`, {
                                        sessionId: msg.sessionId,
                                        dataCase: msg.data?.case,
                                        serializedLength: serialized.length,
                                        envelopeLength: 5 + serialized.length,
                                        readyState: stream.socket.readyState,
                                        bufferedAmount: stream.socket.bufferedAmount
                                    });
                                }
                                const msgBytes = encodeEnvelope(0, serialized);
                                stream.socket.send(msgBytes);
                            }
                            if (isDebugEnabled()) {
                                console.log("[WebSocket] Message iterator completed, sending EndStream");
                            }
                            // CRITICAL: Send EndStream envelope to signal graceful close
                            // Without this, the server detects an abnormal WebSocket closure (code 1005)
                            // and cannot send EndStreamResponse, causing "missing EndStreamResponse" error
                            const endStreamEnvelope = encodeEnvelope(end_stream/* endStreamFlag */.Cu, new Uint8Array(0));
                            stream.socket.send(endStreamEnvelope);
                            if (isDebugEnabled()) {
                                console.log("[WebSocket] EndStream sent successfully");
                            }
                        } catch (err) {
                            console.error("[WebSocket] Error sending input messages:", err);
                        }
                    })();
                    // Parse response headers from first message
                    const headerMsg = await stream.source.next();
                    const connectHeaders = parseHeaders(new TextDecoder().decode(headerMsg.value));
                    // Validate response
                    const status = connectHeaders.get("Status-Code") ?? "-1";
                    const statusCode = parseInt(status);
                    if (statusCode !== 200) {
                        const code = statusCode === 401 ? esm_code/* Code */.C.Unauthenticated : esm_code/* Code */.C.Unknown;
                        throw new connect_error/* ConnectError */.T(`WebSocket response status: ${statusCode}`, code);
                    }
                    const trailer = new Headers();
                    const res = {
                        ...req,
                        header: connectHeaders,
                        trailer,
                        message: parseResponseBody(stream.source, trailer, connectHeaders, ()=>wsCloseCode)
                    };
                    return res;
                }
            });
        }
    };
}
// Parse HTTP headers from string format
function parseHeaders(allHeaders) {
    return allHeaders.trim().split(/[\r\n]+/).reduce((memo, header)=>{
        const [key, value] = header.split(": ");
        if (key && value) {
            memo.append(key, value);
        }
        return memo;
    }, new Headers());
}
// Create readable stream from async generator for envelope parsing
function createEnvelopeReadableStreamForWS(stream) {
    let reader;
    let buffer = new Uint8Array(0);
    function append(chunk) {
        const n = new Uint8Array(buffer.length + chunk.length);
        n.set(buffer);
        n.set(chunk, buffer.length);
        buffer = n;
    }
    return new ReadableStream({
        start () {
            reader = stream;
        },
        async pull (controller) {
            let header;
            for(;;){
                // Try to parse header (5 bytes)
                if (header === undefined && buffer.byteLength >= 5) {
                    let length = 0;
                    for(let i = 1; i < 5; i++){
                        length = (length << 8) + buffer[i];
                    }
                    header = {
                        flags: buffer[0],
                        length
                    };
                }
                // Check if we have full message
                if (header !== undefined && buffer.byteLength >= header.length + 5) {
                    break;
                }
                // Read more data
                const result = await reader.next();
                if (result.done) {
                    break;
                }
                append(result.value);
            }
            if (header === undefined) {
                if (buffer.byteLength === 0) {
                    controller.close();
                    return;
                }
                controller.error(new connect_error/* ConnectError */.T("premature end of stream", esm_code/* Code */.C.DataLoss));
                return;
            }
            // Extract message data
            const data = buffer.subarray(5, 5 + header.length);
            buffer = buffer.subarray(5 + header.length);
            controller.enqueue({
                flags: header.flags,
                data
            });
        }
    });
}

// EXTERNAL MODULE: ./src/lib/config.ts
var config = __webpack_require__(99706);
// EXTERNAL MODULE: ./src/lib/utils/backoff.ts
var backoff = __webpack_require__(6560);
;// ./src/lib/terminal/MessageQueue.ts
/**
 * MessageQueue - Async-iterable queue for outgoing terminal messages.
 *
 * Bridges React callbacks to a ConnectRPC bidirectional stream.
 * The queue implements the async iterator protocol so it can be
 * directly passed to `client.streamTerminal(queue)`.
 *
 * Usage:
 * ```typescript
 * const queue = new MessageQueue();
 * // Producer side
 * queue.push(new TerminalData({ sessionId, data: { case: "input", value: ... } }));
 * // Consumer side (ConnectRPC stream)
 * const stream = client.streamTerminal(queue);
 * // Shutdown
 * queue.close();
 * ```
 */ 

class MessageQueue {
    push(msg) {
        if (this.closed) return;
        if (this.resolve) {
            this.resolve(msg);
            this.resolve = null;
        } else {
            this.queue.push(msg);
        }
    }
    async *[Symbol.asyncIterator]() {
        while(!this.closed){
            if (this.queue.length > 0) {
                yield this.queue.shift();
            } else {
                const msg = await new Promise((resolve)=>{
                    this.resolve = resolve;
                });
                // Don't yield sentinel messages (empty messages used to unblock iterator)
                if (msg.sessionId !== "" || msg.data.case !== undefined) {
                    yield msg;
                }
            }
        }
    }
    /**
   * Closes the queue. Any messages still buffered in `this.queue` (pushed but
   * not yet delivered to an awaiting iterator) are discarded, not drained —
   * the async iterator will yield nothing further for them. Returns the
   * number of messages dropped, so callers can surface a user-facing signal
   * (see `useTerminalStream`'s `onInputDropped`).
   */ close() {
        const droppedCount = this.queue.length;
        this.queue = [];
        this.closed = true;
        if (this.resolve) {
            // Force unblock the iterator with a sentinel message
            // This message will be filtered out by the iterator and not sent to the server
            this.resolve((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                sessionId: "",
                data: {
                    case: undefined
                }
            }));
            this.resolve = null;
        }
        return droppedCount;
    }
    isClosed() {
        return this.closed;
    }
    constructor(){
        this.queue = [];
        this.resolve = null;
        this.closed = false;
    }
}

// EXTERNAL MODULE: ./src/lib/contexts/FeatureFlagsContext.tsx
var FeatureFlagsContext = __webpack_require__(79577);
// EXTERNAL MODULE: ./src/lib/pane/paneUtils.ts
var paneUtils = __webpack_require__(81346);
// EXTERNAL MODULE: ./src/lib/terminal/types.ts
var types = __webpack_require__(47462);
// EXTERNAL MODULE: ./src/lib/terminal/mobileDebug.ts
var mobileDebug = __webpack_require__(28582);
;// ./src/lib/hooks/useResizeSettling.ts
/* __next_internal_client_entry_do_not_use__ useResizeSettling auto */ 


const THROTTLE_MS = 200;
// Last BOUNCE_HISTORY_SIZE sizes sent (oldest first; the current lastSentDimsRef is
// excluded, handled by value-dedup separately). Lets resize() catch a repeat of any
// recent size, not just two sends back, since a real oscillating viewport can cycle
// through 3+ values (observed live: 10x6 -> 67x38 -> 67x22 -> 67x38).
const BOUNCE_HISTORY_SIZE = 4;
const BOUNCE_HOLD_BASE_MS = 3000;
const BOUNCE_HOLD_MAX_MS = 15000;
function isDuplicateOfLastSent(lastSent, next) {
    return lastSent !== null && (0,types/* dimensionsEqual */.k)(lastSent, next);
}
function matchesBounceHistory(history, next) {
    return history.some((d)=>(0,types/* dimensionsEqual */.k)(d, next));
}
function nextBounceHoldMs(streak) {
    return Math.min(BOUNCE_HOLD_BASE_MS * 2 ** streak, BOUNCE_HOLD_MAX_MS);
}
/**
 * useResizeSettling -- the single "resize settling" boundary for the
 * terminal resize pipeline (BUG-101). Owns debounce, oscillation (bounce)
 * detection, and escalating backoff in one place, consolidating logic that
 * was previously patched independently and repeatedly at the resize
 * send-path (see BUG-101 doc): `20da32505`, `417d370dc`/`e5a42e730`,
 * `9651edd5e`, `e8060fb1b`/`b5dba385e`, and the generalized bounce-history
 * fix. Any future oscillation shape belongs here, not as another special
 * case bolted onto a call site.
 */ function useResizeSettling({ onSettled }) {
    const lastResizeTimeRef = (0,react.useRef)(0);
    const lastSentDimsRef = (0,react.useRef)(null);
    const sentHistoryRef = (0,react.useRef)([]);
    // Consecutive bounces held back-to-back with no genuinely-new size sent in
    // between. Drives an escalating hold (doubling, capped) so a viewport
    // stuck oscillating for an extended period backs off further apart instead
    // of retrying every flat hold duration -- resets to 0 the moment a resize
    // actually sends (a real new size, or a held one whose hold elapsed).
    const bounceStreakRef = (0,react.useRef)(0);
    const pendingResizeTimerRef = (0,react.useRef)(null);
    // Cancel any pending deferred resize timer on unmount so it doesn't fire
    // against a torn-down component/connection.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        return ()=>{
            if (pendingResizeTimerRef.current) {
                clearTimeout(pendingResizeTimerRef.current);
                pendingResizeTimerRef.current = null;
            }
        };
    }, []);
    const doSend = (0,react.useCallback)((cols, rows)=>{
        const succeeded = onSettled(cols, rows);
        if (!succeeded) return;
        lastResizeTimeRef.current = Date.now();
        if (lastSentDimsRef.current !== null) {
            sentHistoryRef.current.push(lastSentDimsRef.current);
            if (sentHistoryRef.current.length > BOUNCE_HISTORY_SIZE) {
                sentHistoryRef.current.shift();
            }
        }
        lastSentDimsRef.current = {
            cols,
            rows
        };
        bounceStreakRef.current = 0;
    }, [
        onSettled
    ]);
    // Defers `doSend(dims)` by delayMs, replacing whatever deferred send (if
    // any) was already pending. Shared by the bounce-hold and trailing-edge-
    // throttle branches of resize() below -- the only difference between them
    // is how delayMs and the log line are computed.
    const scheduleDeferredSend = (0,react.useCallback)((dims, delayMs)=>{
        pendingResizeTimerRef.current = setTimeout(()=>{
            pendingResizeTimerRef.current = null;
            doSend(dims.cols, dims.rows);
        }, delayMs);
    }, [
        doSend
    ]);
    const resize = (0,react.useCallback)((cols, rows, force = false, opts)=>{
        const next = {
            cols,
            rows
        };
        const bypassed = opts?.bypassBounceHold === true;
        // Cancel any previously deferred resize -- we have newer dimensions now.
        // This MUST run before the value-dedup early-return below: otherwise a
        // bounce-back call whose dimensions match lastSentDimsRef (e.g. A -> B
        // deferred within the throttle window -> back to A) would dedup-return
        // without cancelling the still-pending deferred send for B, letting
        // that stale send fire later with the wrong dimensions.
        if (pendingResizeTimerRef.current) {
            clearTimeout(pendingResizeTimerRef.current);
            pendingResizeTimerRef.current = null;
        }
        // Value-dedup: skip if this exact (cols, rows) pair was the last one
        // actually sent, independent of (and checked before) the time throttle
        // below. An unchanged value must not keep the throttle window "warm" --
        // lastResizeTimeRef is deliberately left untouched here.
        if (!force && isDuplicateOfLastSent(lastSentDimsRef.current, next)) {
            console.log(`[useResizeSettling] Resize skipped, value unchanged (${cols}x${rows})`);
            mobileDebug/* mobileDebug */.Uo.log('resize', {
                cols,
                rows,
                outcome: 'deduped',
                bypassed
            });
            return;
        }
        // Bounce detection: matches ANY of the last BOUNCE_HISTORY_SIZE sizes
        // sent, not just the one two sends ago, since a real oscillating
        // viewport can wander through 3+ values on a slower cadence than
        // THROTTLE_MS catches. Held out past an escalating hold instead of
        // sent immediately, coalescing the oscillation into one settled resize.
        if (!force && !bypassed && matchesBounceHistory(sentHistoryRef.current, next)) {
            const holdMs = nextBounceHoldMs(bounceStreakRef.current);
            bounceStreakRef.current += 1;
            console.log(`[useResizeSettling] Resize bounce detected (${cols}x${rows} matches recent history), holding ${holdMs}ms (streak ${bounceStreakRef.current})`);
            mobileDebug/* mobileDebug */.Uo.log('resize', {
                cols,
                rows,
                outcome: 'bounce',
                bounce: true,
                holdMs,
                streak: bounceStreakRef.current,
                bypassed
            });
            scheduleDeferredSend(next, holdMs);
            return;
        }
        const timeSinceLastResize = Date.now() - lastResizeTimeRef.current;
        if (!force && timeSinceLastResize < THROTTLE_MS && lastResizeTimeRef.current !== 0) {
            // Defer instead of drop: schedule the trailing-edge send so the final
            // settled size always reaches the server after rapid resize sequences.
            const remaining = THROTTLE_MS - timeSinceLastResize;
            console.log(`[useResizeSettling] Resize deferred ${remaining}ms (${cols}x${rows})`);
            mobileDebug/* mobileDebug */.Uo.log('resize', {
                cols,
                rows,
                outcome: 'deferred',
                bounce: false,
                holdMs: remaining,
                bypassed
            });
            scheduleDeferredSend(next, remaining + 1);
            return;
        }
        mobileDebug/* mobileDebug */.Uo.log('resize', {
            cols,
            rows,
            outcome: 'sent',
            bounce: false,
            bypassed
        });
        doSend(cols, rows);
    }, [
        doSend,
        scheduleDeferredSend
    ]);
    return {
        resize
    };
}

;// ./src/lib/hooks/useTerminalFlowControl.ts
/* __next_internal_client_entry_do_not_use__ RESYNC_CORRELATION_ID_FLAG,useTerminalFlowControl auto */ 





// Epic 3.1 (AC2) — client-generated correlation ID echoed back on the
// resulting TerminalOutput.resync_id. Flag off preserves pre-project wire
// behavior: CurrentPaneRequest.resync_id is left empty and
// requestFullResync()'s return value is always undefined.
const RESYNC_CORRELATION_ID_FLAG = 'terminal:resync-correlation-id';
/**
 * useTerminalFlowControl - Resync logic, resize throttling, and message dispatch
 * for terminal input/resize/scrollback/flow-control.
 */ function useTerminalFlowControl({ sessionId, getTerminal, pushMessageRef, isConnectedRef, onError, outstandingResyncIdsRef, readOnly = false }) {
    const correlationIdEnabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)(RESYNC_CORRELATION_ID_FLAG);
    const readOnlyRef = (0,react.useRef)(readOnly);
    readOnlyRef.current = readOnly;
    // Resync state machine refs. Hold the pending resync's generated tracking
    // ID (null when no resync is pending) rather than a plain boolean — Epic
    // 3.1, Task 3.1.1.2. Truthiness is preserved for existing consumers
    // (useTerminalStream.ts's disconnect()): a non-empty string is truthy,
    // null is falsy, so the existing `if (isResyncingRef.current)` checks keep
    // working unchanged. An internal tracking ID is always generated
    // (independent of the correlation-ID flag) so this pending-state machinery
    // behaves identically whether or not the flag is on; only the wire
    // encoding (CurrentPaneRequest.resync_id) and requestFullResync's return
    // value are gated on the flag.
    const isResyncingRef = (0,react.useRef)(null);
    const waitingForPaneResponseRef = (0,react.useRef)(null);
    const lastResyncTimeRef = (0,react.useRef)(0);
    // Ref, not state: the gesture rAF callback must read the live value.
    // One token per in-flight chunked paste, so an overlapping or aborted paste never clears another's flag.
    const activePastesRef = (0,react.useRef)(new Set());
    const sessionIdRef = (0,react.useRef)(sessionId);
    sessionIdRef.current = sessionId;
    const paneRequestTimerRef = (0,react.useRef)(null);
    const dimensionSyncRef = (0,react.useRef)({});
    // Epic 3.1, Task 3.1.1.4a — last dimensions a resync response was actually
    // applied for (set in markPaneResponseReceived). Compared against the
    // terminal's CURRENT dimensions in requestFullResync to compute
    // stale_dimensions: null until the first resync response lands, so the
    // flag defaults to false on a cold start rather than an unconditional true
    // (pre-mortem P1 risk — must not reintroduce the corruption bug an
    // always-true stale_dimensions would cause).
    const lastSyncedDimensionsRef = (0,react.useRef)(null);
    // Cancel any pending deferred resize/pane-request timers when the component
    // unmounts to prevent the timer callback from firing against a torn-down
    // component/connection.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        return ()=>{
            if (paneRequestTimerRef.current) {
                clearTimeout(paneRequestTimerRef.current);
                paneRequestTimerRef.current = null;
            }
        };
    }, []);
    // Helper to set error state
    const handleError = (0,react.useCallback)((err)=>{
        const error = err instanceof Error ? err : new Error(String(err));
        onError?.(error);
    }, [
        onError
    ]);
    // Helper to push a message (reads from ref to avoid stale closures)
    const pushMessage = (0,react.useCallback)((msg)=>{
        pushMessageRef.current?.(msg);
    }, [
        pushMessageRef
    ]);
    // Shared connection gate for every public dispatch function below — a copy-pasted
    // per-callsite check is how sendInput's copy silently lacked a console.warn. Internal
    // per-chunk/per-tick continuation checks stay inline (a background retry after the
    // initial call already logged once shouldn't log again).
    const ensureConnected = (0,react.useCallback)((action)=>{
        if (!pushMessageRef.current || !isConnectedRef.current) {
            console.warn(`[useTerminalFlowControl] Cannot ${action}: stream not connected`);
            return false;
        }
        return true;
    }, [
        pushMessageRef,
        isConnectedRef
    ]);
    // ---- Resync ----
    const requestFullResync = (0,react.useCallback)((urgent = false, isVisibilityTriggered = false)=>{
        if (!ensureConnected("request resync")) {
            return undefined;
        }
        const currentTerminal = getTerminal();
        if (!currentTerminal) {
            console.warn("[useTerminalFlowControl] Cannot request resync: terminal not available");
            return undefined;
        }
        const now = Date.now();
        const timeSinceLastResync = now - lastResyncTimeRef.current;
        const RESYNC_THROTTLE_MS = 2000;
        if (!urgent && timeSinceLastResync < RESYNC_THROTTLE_MS && lastResyncTimeRef.current !== 0) {
            console.log(`[useTerminalFlowControl] Resync throttled (${timeSinceLastResync}ms since last, need ${RESYNC_THROTTLE_MS}ms)`);
            return undefined;
        }
        if (urgent) {
            console.log(`[useTerminalFlowControl] Urgent resync bypassing throttle (${timeSinceLastResync}ms since last)`);
        }
        try {
            console.log(`[useTerminalFlowControl] Requesting full resync with current dimensions: ${currentTerminal.cols}x${currentTerminal.rows}`);
            lastResyncTimeRef.current = now;
            const trackingId = (0,paneUtils/* generateSecureId */.cg)();
            isResyncingRef.current = trackingId;
            waitingForPaneResponseRef.current = trackingId;
            dimensionSyncRef.current = {
                cols: currentTerminal.cols,
                rows: currentTerminal.rows
            };
            // Epic 3.1, Task 3.1.1.4b — only a visibility/focus-triggered resync
            // can be "stale" in this sense (a resize-triggered one always carries
            // just-measured dimensions). Defaults to false until a resync response
            // has actually been applied at least once (lastSyncedDimensionsRef
            // starts null), and correctly stays false when the terminal's own
            // dimensions genuinely changed while backgrounded (currentTerminal.cols/
            // rows would then differ from lastSyncedDimensionsRef).
            const staleDimensions = isVisibilityTriggered && lastSyncedDimensionsRef.current !== null && currentTerminal.cols === lastSyncedDimensionsRef.current.cols && currentTerminal.rows === lastSyncedDimensionsRef.current.rows;
            const resyncId = correlationIdEnabled ? trackingId : "";
            const currentPaneReq = (0,create/* create */.v)(events_pb/* CurrentPaneRequestSchema */.kq, {
                lines: 50,
                includeEscapes: true,
                targetCols: currentTerminal.cols,
                targetRows: currentTerminal.rows,
                resyncId,
                staleDimensions
            });
            if (resyncId && outstandingResyncIdsRef) {
                outstandingResyncIdsRef.current.set(resyncId, Date.now());
            }
            pushMessage((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                sessionId,
                data: {
                    case: "currentPaneRequest",
                    value: currentPaneReq
                }
            }));
            return resyncId || undefined;
        } catch (err) {
            handleError(err);
            return undefined;
        }
    }, [
        sessionId,
        getTerminal,
        pushMessage,
        ensureConnected,
        handleError,
        correlationIdEnabled,
        outstandingResyncIdsRef
    ]);
    // ---- Message dispatch functions ----
    const PASTE_CHUNK_SIZE = 512; // bytes — fits within tmux's pty write buffer
    const CHUNK_DELAY_MS = 10; // ms between chunks — yields event loop without stalling input
    const sendInput = (0,react.useCallback)((input)=>{
        if (readOnlyRef.current) return;
        if (!ensureConnected("send input")) return;
        const encoder = new TextEncoder();
        const inputBytes = encoder.encode(input);
        if (inputBytes.length <= PASTE_CHUNK_SIZE) {
            try {
                pushMessage((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                    sessionId,
                    data: {
                        case: "input",
                        value: (0,create/* create */.v)(events_pb/* TerminalInputSchema */.Qn, {
                            data: inputBytes
                        })
                    }
                }));
            } catch (err) {
                handleError(err);
            }
            return;
        }
        // Large input: send in chunks to avoid tmux/WebSocket buffer limits.
        // Capture sessionId at call-time — if the session changes mid-paste the
        // pending chunks are aborted (sessionIdAtStart !== current sessionId).
        const sessionIdAtStart = sessionId;
        let offset = 0;
        const pasteToken = Symbol("paste");
        const activePastes = activePastesRef.current;
        const finishPaste = ()=>{
            activePastes.delete(pasteToken);
        };
        const sendChunk = ()=>{
            // sessionIdRef, not the closed-over sessionId, which is always sessionIdAtStart.
            if (!pushMessageRef.current || !isConnectedRef.current || sessionIdRef.current !== sessionIdAtStart || // session changed; abort
            offset >= inputBytes.length) {
                finishPaste();
                return;
            }
            const chunk = inputBytes.slice(offset, offset + PASTE_CHUNK_SIZE);
            offset += PASTE_CHUNK_SIZE;
            try {
                pushMessage((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                    sessionId: sessionIdAtStart,
                    data: {
                        case: "input",
                        value: (0,create/* create */.v)(events_pb/* TerminalInputSchema */.Qn, {
                            data: chunk
                        })
                    }
                }));
            } catch (err) {
                finishPaste();
                handleError(err);
                return;
            }
            if (offset < inputBytes.length) {
                activePastes.add(pasteToken);
                setTimeout(sendChunk, CHUNK_DELAY_MS);
            } else {
                finishPaste();
            }
        };
        sendChunk();
    }, [
        sessionId,
        pushMessage,
        pushMessageRef,
        isConnectedRef,
        handleError,
        ensureConnected
    ]);
    // True from the first chunk of a >512 B paste until the last chunk is sent (or it aborts).
    const isInputChunking = (0,react.useCallback)(()=>activePastesRef.current.size > 0, []);
    // A session switch aborts the pending chunks; clear eagerly rather than waiting for the next timer.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        activePastesRef.current.clear();
    }, [
        sessionId
    ]);
    // The actual send, invoked by useResizeSettling only once it has judged a
    // (cols, rows) value settled (BUG-101) — debounce, throttle, and
    // oscillation/bounce detection all live in that hook now, not here. Stamps
    // the timestamp, sends the resize RPC, then requests a fresh pane capture
    // so xterm.js and tmux are guaranteed to agree on content. Returns whether
    // the send succeeded so useResizeSettling knows whether to record this
    // value as the last one actually sent.
    const sendSettledResize = (0,react.useCallback)((cols, rows)=>{
        if (!pushMessageRef.current || !isConnectedRef.current) return false;
        if (!readOnlyRef.current) {
            try {
                console.log(`[useTerminalFlowControl] Sending resize to server: ${cols}x${rows}`);
                pushMessage((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                    sessionId,
                    data: {
                        case: "resize",
                        value: (0,create/* create */.v)(events_pb/* TerminalResizeSchema */.GF, {
                            cols,
                            rows
                        })
                    }
                }));
            } catch (err) {
                handleError(err);
                return false;
            }
        }
        // After resizing, request fresh terminal content
        paneRequestTimerRef.current = setTimeout(()=>{
            paneRequestTimerRef.current = null;
            if (!pushMessageRef.current || !isConnectedRef.current) return;
            try {
                console.log(`[useTerminalFlowControl] Requesting fresh pane content after resize`);
                // Epic 3.1, Task 3.1.1.3 — a resize-triggered pane request always
                // carries just-measured dimensions, so stale_dimensions is always
                // explicitly false here (never the isVisibilityTriggered formula
                // used in requestFullResync).
                const resyncId = correlationIdEnabled ? (0,paneUtils/* generateSecureId */.cg)() : "";
                if (resyncId && outstandingResyncIdsRef) {
                    outstandingResyncIdsRef.current.set(resyncId, Date.now());
                }
                pushMessage((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                    sessionId,
                    data: {
                        case: "currentPaneRequest",
                        value: (0,create/* create */.v)(events_pb/* CurrentPaneRequestSchema */.kq, {
                            lines: 50,
                            includeEscapes: true,
                            targetCols: cols,
                            targetRows: rows,
                            resyncId,
                            staleDimensions: false
                        })
                    }
                }));
            } catch (err) {
                handleError(err);
            }
        }, 100);
        return true;
    }, [
        sessionId,
        pushMessage,
        pushMessageRef,
        isConnectedRef,
        handleError,
        correlationIdEnabled,
        outstandingResyncIdsRef
    ]);
    const { resize: settledResize } = useResizeSettling({
        onSettled: sendSettledResize
    });
    const resize = (0,react.useCallback)((cols, rows, force = false, opts)=>{
        if (!ensureConnected("resize terminal")) return;
        settledResize(cols, rows, force, opts);
    }, [
        ensureConnected,
        settledResize
    ]);
    const requestScrollback = (0,react.useCallback)((fromSequence, limit)=>{
        if (!ensureConnected("request scrollback")) return;
        try {
            console.log(`[useTerminalFlowControl] Requesting scrollback: fromSeq=${fromSequence}, limit=${limit}`);
            pushMessage((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                sessionId,
                data: {
                    case: "scrollbackRequest",
                    value: (0,create/* create */.v)(events_pb/* ScrollbackRequestSchema */.pF, {
                        fromSequence: BigInt(fromSequence),
                        limit
                    })
                }
            }));
        } catch (err) {
            handleError(err);
        }
    }, [
        sessionId,
        pushMessage,
        ensureConnected,
        handleError
    ]);
    const sendFlowControl = (0,react.useCallback)((paused, watermark)=>{
        if (!ensureConnected("send flow control")) return;
        try {
            console.log(`[useTerminalFlowControl] Sending flow control: paused=${paused}, watermark=${watermark || 'N/A'}`);
            pushMessage((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                sessionId,
                data: {
                    case: "flowControl",
                    value: (0,create/* create */.v)(events_pb/* FlowControlSchema */.bx, {
                        paused,
                        watermark: watermark !== undefined ? BigInt(watermark) : undefined
                    })
                }
            }));
        } catch (err) {
            handleError(err);
        }
    }, [
        sessionId,
        pushMessage,
        ensureConnected,
        handleError
    ]);
    const markResyncComplete = (0,react.useCallback)(()=>{
        isResyncingRef.current = null;
    }, []);
    const markPaneResponseReceived = (0,react.useCallback)(()=>{
        waitingForPaneResponseRef.current = null;
        // Epic 3.1, Task 3.1.1.4a — this is the single existing "resync response
        // applied" signal (invoked, via useVisibilityResync.ts's
        // notifyResyncOutputReceived, whenever a visibility/focus-triggered
        // resync completes), so it's the correct integration point to record
        // which dimensions the client believes are now in sync with the server.
        if (dimensionSyncRef.current.cols !== undefined && dimensionSyncRef.current.rows !== undefined) {
            lastSyncedDimensionsRef.current = {
                cols: dimensionSyncRef.current.cols,
                rows: dimensionSyncRef.current.rows
            };
        }
    }, []);
    const getIsResyncingRef = (0,react.useCallback)(()=>isResyncingRef, []);
    const getWaitingForPaneResponseRef = (0,react.useCallback)(()=>waitingForPaneResponseRef, []);
    return {
        sendInput,
        isInputChunking,
        resize,
        requestScrollback,
        sendFlowControl,
        requestFullResync,
        markResyncComplete,
        markPaneResponseReceived,
        getIsResyncingRef,
        getWaitingForPaneResponseRef
    };
}

;// ./src/lib/hooks/useTerminalMetrics.ts
/* __next_internal_client_entry_do_not_use__ useTerminalMetrics auto */ 
/**
 * useTerminalMetrics - RAF-based output batching and debug recording.
 *
 * Owns the adaptive requestAnimationFrame batching of terminal output
 * and the WebSocket message recording feature for debugging.
 *
 * Output batching strategy:
 * - Uses requestAnimationFrame for display-synchronized updates (60-144fps)
 * - Flushes immediately if buffer exceeds 4KB (prevents lag on large bursts)
 * - Automatically adapts to high refresh rate displays
 */ function useTerminalMetrics({ onOutput }) {
    const [output, setOutput] = (0,react.useState)("");
    // RAF-based output batching refs
    const outputBufferRef = (0,react.useRef)([]);
    const pendingUpdateRef = (0,react.useRef)(null);
    const textDecoderRef = (0,react.useRef)(new TextDecoder());
    const bufferSizeRef = (0,react.useRef)(0);
    // Recording refs
    const recordedMessagesRef = (0,react.useRef)([]);
    const isRecordingRef = (0,react.useRef)(false);
    const [isRecording, setIsRecording] = (0,react.useState)(false);
    // Flush buffered output to state
    const flushOutputBuffer = (0,react.useCallback)(()=>{
        if (outputBufferRef.current.length > 0) {
            const bufferedText = outputBufferRef.current.join("");
            outputBufferRef.current = [];
            bufferSizeRef.current = 0;
            setOutput((prev)=>prev + bufferedText);
        }
        pendingUpdateRef.current = null;
    }, []);
    // Schedule output update with adaptive batching
    const scheduleOutputUpdate = (0,react.useCallback)((text)=>{
        // If onOutput callback provided, use it directly (better performance)
        if (onOutput) {
            onOutput(text);
            return;
        }
        // Fallback: batch via RAF into React state
        outputBufferRef.current.push(text);
        bufferSizeRef.current += text.length;
        // Immediate flush for large buffers (>4KB)
        if (bufferSizeRef.current > 4096) {
            if (pendingUpdateRef.current !== null) {
                cancelAnimationFrame(pendingUpdateRef.current);
                pendingUpdateRef.current = null;
            }
            flushOutputBuffer();
            return;
        }
        // Otherwise, use RAF for display-synchronized batching
        if (pendingUpdateRef.current === null) {
            pendingUpdateRef.current = requestAnimationFrame(flushOutputBuffer);
        }
    }, [
        flushOutputBuffer,
        onOutput
    ]);
    // Recording functions
    const startRecording = (0,react.useCallback)(()=>{
        recordedMessagesRef.current = [];
        isRecordingRef.current = true;
        setIsRecording(true);
        console.log('[Recording] Started terminal output recording');
    }, []);
    const stopRecording = (0,react.useCallback)(()=>{
        isRecordingRef.current = false;
        setIsRecording(false);
        const blob = new Blob([
            JSON.stringify(recordedMessagesRef.current, null, 2)
        ], {
            type: 'application/json'
        });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `terminal-recording-${Date.now()}.json`;
        a.click();
        console.log('[Recording] Saved recording with', recordedMessagesRef.current.length, 'messages');
    }, []);
    const recordMessage = (0,react.useCallback)((msg)=>{
        if (isRecordingRef.current) {
            recordedMessagesRef.current.push(msg);
        }
    }, []);
    // Cleanup on unmount: cancel pending RAF, flush remaining output
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        return ()=>{
            if (pendingUpdateRef.current !== null) {
                cancelAnimationFrame(pendingUpdateRef.current);
                pendingUpdateRef.current = null;
            }
            // Flush any remaining buffered output
            if (outputBufferRef.current.length > 0) {
                const bufferedText = outputBufferRef.current.join("");
                outputBufferRef.current = [];
                bufferSizeRef.current = 0;
                setOutput((prev)=>prev + bufferedText);
            }
        };
    }, []);
    return {
        output,
        scheduleOutputUpdate,
        flushOutputBuffer,
        startRecording,
        stopRecording,
        recordMessage,
        isRecording
    };
}

// EXTERNAL MODULE: ./src/gen/session/v1/types_pb.ts
var types_pb = __webpack_require__(29238);
;// ./src/lib/hooks/useTerminalStream.ts
/* __next_internal_client_entry_do_not_use__ useTerminalStream auto */ 











function useTerminalStream({ baseUrl, sessionId, shellId, onShellStatusChange, getTerminal, scrollbackLines = 1000, onError, onScrollbackReceived, onOutput, onAppScrollback, onAltScreenActiveHint, autoConnect = true, initialCols, initialRows, onInputDropped, foreground = false, outstandingResyncIdsRef, readOnly = false }) {
    // ---- Connection state ----
    const [isConnected, setIsConnected] = (0,react.useState)(false);
    const [error, setError] = (0,react.useState)(null);
    const [scrollbackLoaded, setScrollbackLoaded] = (0,react.useState)(false);
    // Task 4.1.1 — Terminal state machine (R1.4)
    const [terminalState, setTerminalState] = (0,react.useState)('DISCONNECTED');
    const [isHardFailed, setIsHardFailed] = (0,react.useState)(false);
    // Epic 4.2, Story 4.2.1 — undefined until a PathHubOwned session's first
    // connection_count-carrying message arrives; never fabricated for
    // PathLegacyPerConnection sessions (proto field is absent there).
    const [connectionCount, setConnectionCount] = (0,react.useState)(undefined);
    const messageQueueRef = (0,react.useRef)(null);
    const abortControllerRef = (0,react.useRef)(null);
    const isDisconnectingRef = (0,react.useRef)(false);
    const isConnectedRef = (0,react.useRef)(false);
    // Guards against two independent visibility/focus-triggered reconnect paths
    // (this hook's own Story 3.1.3 listener and useVisibilityResync, composed
    // together in TerminalOutput.tsx) both calling connect() for the same
    // disconnected session before either handshake completes.
    const isConnectingRef = (0,react.useRef)(false);
    const shouldReconnectRef = (0,react.useRef)(false);
    const terminalBackoffRef = (0,react.useRef)(new backoff/* BackoffState */.Et(1000, 30000));
    const isHardFailedRef = (0,react.useRef)(false);
    const terminalDebounceTimerRef = (0,react.useRef)(null);
    const reconnectTimerRef = (0,react.useRef)(null);
    const connectTimeoutRef = (0,react.useRef)(null);
    const foregroundRef = (0,react.useRef)(foreground);
    const foregroundConnectAttemptRef = (0,react.useRef)(0);
    // Shared across the hook's whole lifetime (not per-connect() call) because the
    // connectTimeoutRef callback below is declared outside the async message-processing
    // IIFE and needs to read it. Correctness relies on connect()'s own re-entrancy guard
    // (isConnectedRef/isConnectingRef) preventing two attempts from touching this ref at
    // once — do not relax those guards without re-checking this invariant.
    const firstMessageRef = (0,react.useRef)(true);
    // Set immediately before attemptController.abort() fires from a connect-timeout,
    // so the resulting stream error can be routed past onError (see the catch block
    // below) instead of surfacing as a visible connection failure — a connect-timeout
    // is a deliberate, internal fast-retry optimization, not a real failure the caller
    // should count toward its own attempt/error UI.
    const connectTimeoutAbortedRef = (0,react.useRef)(false);
    const connectRef = (0,react.useRef)(async ()=>{});
    const textDecoderRef = (0,react.useRef)(new TextDecoder());
    const scrollbackDecoderRef = (0,react.useRef)(new TextDecoder());
    // Task 2.2.1 — Connection-generation fence (mirrors usePathCompletions.ts's
    // generationRef idiom). Bumped once per connect() call; a message-processing
    // loop whose captured generation no longer matches the current value treats
    // itself as superseded and stops mutating shared state.
    const connectionGenerationRef = (0,react.useRef)(0);
    const clientRef = (0,react.useRef)((0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, createWebsocketBasedTransport({
        baseUrl,
        useBinaryFormat: true,
        interceptors: [
            (0,config/* createAuthInterceptor */._)()
        ]
    })));
    // Clears the pending per-attempt connect-timeout timer, if any. Shared across every
    // exit path (first-message success, stream error/end, disconnect(), unmount, and a
    // synchronous throw before the async message loop starts) so none of them can miss it.
    const clearConnectTimeout = (0,react.useCallback)(()=>{
        if (connectTimeoutRef.current) {
            clearTimeout(connectTimeoutRef.current);
            connectTimeoutRef.current = null;
        }
    }, []);
    // Sync ref with state
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        isConnectedRef.current = isConnected;
    }, [
        isConnected
    ]);
    // Detect the foreground false→true transition (AC3): reset both the
    // existing backoff-delay counter and the new fast-connect-timeout
    // attempt counter so a just-selected terminal gets the full fast
    // window immediately, not whatever was left over from before it was
    // foreground (or from background attempts that happened while it wasn't selected).
    //
    // Also clear any pending reconnectTimerRef and reconnect immediately (pre-mortem
    // Failure #1, P1): without this, a terminal that was mid-backoff-delay while
    // backgrounded would still sit out that stale, potentially-30s wait after being
    // selected, before the fast connect-timeout ever got a chance to apply.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const wasForeground = foregroundRef.current;
        foregroundRef.current = foreground;
        if (process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true") return; // same gate as the rest of this file's auto-reconnect logic
        if (!wasForeground && foreground) {
            terminalBackoffRef.current.reset();
            foregroundConnectAttemptRef.current = 0;
            if (reconnectTimerRef.current) {
                clearTimeout(reconnectTimerRef.current);
                reconnectTimerRef.current = null;
                // !isDisconnectingRef.current is unreachable via any path today — disconnect()
                // always clears reconnectTimerRef before setting isDisconnectingRef, so this
                // branch can never observe both truthy at once — but it's cheap, matches
                // handleVisibilityOrOnline's analogous guard below, and protects a future
                // refactor of disconnect()'s ordering from silently reintroducing a race.
                // Not independently unit-tested for that reason (see sdd:6-verify PR review).
                if (shouldReconnectRef.current && !isConnectingRef.current && !isConnectedRef.current && !isDisconnectingRef.current) {
                    connectRef.current?.();
                }
            }
        }
    }, [
        foreground
    ]);
    // ---- Compose sub-hooks ----
    // pushMessageRef bridges the connection's messageQueue to flow control dispatch.
    // Bug Risk 3 mitigation: flow control reads pushMessageRef.current (not a stale closure).
    const pushMessageRef = (0,react.useRef)(null);
    // Keep pushMessageRef in sync with current messageQueue
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        pushMessageRef.current = (msg)=>{
            messageQueueRef.current?.push(msg);
        };
        return ()=>{
            pushMessageRef.current = null;
        };
    }, []);
    const flowControl = useTerminalFlowControl({
        sessionId,
        getTerminal: getTerminal ?? (()=>null),
        pushMessageRef,
        isConnectedRef,
        onError,
        outstandingResyncIdsRef,
        readOnly
    });
    const metrics = useTerminalMetrics({
        onOutput
    });
    // ---- Error helper ----
    const handleError = (0,react.useCallback)((err)=>{
        const error = err instanceof Error ? err : new Error(String(err));
        setError(error);
        onError?.(error);
    }, [
        onError
    ]);
    // ---- Connect ----
    const connect = (0,react.useCallback)(async (overrideCols, overrideRows, options)=>{
        // Refuse to reconnect through a hard failure. The only sanctioned way back
        // in is handleManualReconnect below, which clears isHardFailedRef.current
        // to false *before* calling connect() — so this check never blocks Retry,
        // only callers (resize handlers, visibility/focus fallbacks, stale mount
        // effects) that invoke connect() directly without going through Retry.
        if (isHardFailedRef.current) return;
        if (isConnectedRef.current || isConnectingRef.current || !sessionId) return;
        isConnectingRef.current = true;
        shouldReconnectRef.current = true;
        // Only the finally block's own self-scheduled automatic retry (below) passes
        // isAutoRetry: true. Every other caller — mount, manual reconnect, the
        // visibility/online listener and foreground-transition effect in this file,
        // and useVisibilityResync's two direct connect() calls — omits it and gets the
        // fresh backoff sequence a genuinely new attempt sequence should have. Without
        // this gate, the automatic retry itself reset terminalBackoffRef on every call,
        // so `.attempt` was always 0 when the hard-fail check below ran and the delay
        // below never escalated past uniform(0, 1000ms).
        if (!options?.isAutoRetry) {
            terminalBackoffRef.current.reset();
        }
        // Task 2.2.1 — bump the connection generation immediately so this call's
        // message-processing loop (started below) can identify itself as "the
        // current attempt" and detect being superseded by a later connect().
        const myGeneration = ++connectionGenerationRef.current;
        const connectStartedAt = Date.now();
        let targetCols = overrideCols ?? initialCols;
        let targetRows = overrideRows ?? initialRows;
        if (targetCols === undefined || targetRows === undefined) {
            const currentTerminal = getTerminal?.();
            if (currentTerminal) {
                targetCols = currentTerminal.cols;
                targetRows = currentTerminal.rows;
                console.log(`[useTerminalStream] Using current terminal dimensions: ${targetCols}x${targetRows}`);
            }
        }
        isDisconnectingRef.current = false;
        setTerminalState('CONNECTING');
        try {
            // Task 2.2.2 — unconditionally tear down whatever generation this call
            // is about to replace, regardless of connection state. This removes the
            // previous isConnectedRef-gated skip (the root of the double-live-
            // connection risk documented in architecture.md §1) by making connect()
            // itself always close/abort what it's about to replace.
            if (messageQueueRef.current) {
                const dropped = messageQueueRef.current.close();
                if (dropped > 0) {
                    onInputDropped?.(dropped);
                }
            }
            if (abortControllerRef.current) {
                abortControllerRef.current.abort();
            }
            abortControllerRef.current = new AbortController();
            const attemptController = abortControllerRef.current;
            firstMessageRef.current = true;
            connectTimeoutAbortedRef.current = false;
            const foregroundAtSchedule = foregroundRef.current;
            const timeoutMs = (0,backoff/* connectTimeoutMs */.Ir)(foregroundAtSchedule, foregroundConnectAttemptRef.current);
            // Counted unconditionally (even with the flag off) since it's cheap and inert
            // when nothing schedules a connect-timeout to consume it.
            foregroundConnectAttemptRef.current += 1;
            const attemptNumber = foregroundConnectAttemptRef.current;
            if (process.env.NEXT_PUBLIC_RECONNECT_V2 === "true") {
                connectTimeoutRef.current = setTimeout(()=>{
                    connectTimeoutRef.current = null;
                    // Race guard (pre-mortem Failure #3, P2): re-check firstMessageRef
                    // immediately before aborting, so a message that already landed and
                    // was processed is not retroactively aborted.
                    if (!firstMessageRef.current) return;
                    console.warn(`[reconnect] stream=terminal trigger=connect-timeout foreground=${foregroundAtSchedule} attempt=${attemptNumber} timeoutMs=${timeoutMs}`);
                    connectTimeoutAbortedRef.current = true;
                    attemptController.abort();
                }, timeoutMs);
            }
            messageQueueRef.current = new MessageQueue();
            // Send initial handshake with dimensions
            const currentPaneReq = (0,create/* create */.v)(events_pb/* CurrentPaneRequestSchema */.kq, {
                lines: 50,
                includeEscapes: true
            });
            if (targetCols !== undefined && targetRows !== undefined) {
                currentPaneReq.targetCols = targetCols;
                currentPaneReq.targetRows = targetRows;
                console.log(`[useTerminalStream] Sending handshake with dimensions: ${targetCols}x${targetRows}`);
            } else {
                console.warn(`[useTerminalStream] No terminal dimensions available for handshake`);
            }
            messageQueueRef.current.push((0,create/* create */.v)(events_pb/* TerminalDataSchema */.Mi, {
                sessionId,
                shellId: shellId ?? "",
                data: {
                    case: "currentPaneRequest",
                    value: currentPaneReq
                }
            }));
            const stream = clientRef.current.streamTerminal(messageQueueRef.current, {
                signal: abortControllerRef.current.signal
            });
            setError(null);
            // Message processing loop
            (async ()=>{
                try {
                    for await (const msg of stream){
                        // Task 2.2.3 — a superseded generation's loop must not mutate
                        // shared state (or, transitively, deliver buffered input to the
                        // wrong connection). Mirrors usePathCompletions.ts's
                        // `if (generation !== generationRef.current) return;` guard.
                        if (myGeneration !== connectionGenerationRef.current) {
                            console.warn(`[useTerminalStream] Discarding message from superseded connection generation ${myGeneration} (current: ${connectionGenerationRef.current})`);
                            break;
                        }
                        if (firstMessageRef.current) {
                            clearConnectTimeout();
                            isConnectingRef.current = false;
                            isConnectedRef.current = true; // sync ref before state setter — mirrors the disconnect-path precedent in the finally block below, closing the window where a same-tick second connect() call could read a stale isConnectedRef before React flushes setIsConnected(true)
                            setIsConnected(true);
                            setScrollbackLoaded(true);
                            setTerminalState('LOADING');
                            firstMessageRef.current = false;
                        }
                        // Task 4.1.2 — Handle ResizeQuiescence message (R1.4).
                        // Transitions: resizing=true → RESIZING, resizing=false → STABLE.
                        if (msg.data.case === "resizeQuiescence") {
                            const rq = msg.data.value;
                            if (rq.resizing) {
                                setTerminalState('RESIZING');
                            } else {
                                setTerminalState('STABLE');
                            }
                            continue; // No further processing for quiescence messages
                        }
                        // Handle ShellStatusUpdate for custom shell PTY streams.
                        // Only fire when msg.shellId matches our shellId (guards against stray updates).
                        if (msg.data.case === "shellStatusUpdate") {
                            const update = msg.data.value;
                            if (onShellStatusChange && update.shellId === shellId) {
                                let statusStr = "running";
                                if (update.newStatus === types_pb/* ShellStatus */.d5.STOPPED) statusStr = "stopped";
                                else if (update.newStatus === types_pb/* ShellStatus */.d5.ERROR) statusStr = "error";
                                onShellStatusChange(statusStr, update.exitCode);
                            }
                            continue;
                        }
                        if (msg.data.case === "output") {
                            // Task 4.2.1c (Epic 4.2) — connection_count rides on TerminalOutput,
                            // including on side-channel messages the server sends with no `data`
                            // (server/services/connectrpc_websocket.go's sendConnectionCountUpdates).
                            // Only ever present for PathHubOwned sessions; undefined otherwise —
                            // never fabricated (plan.md Story 4.2.1 AC2).
                            if (msg.data.value.connectionCount !== undefined) {
                                setConnectionCount(msg.data.value.connectionCount);
                            }
                            // See onAltScreenActiveHint's doc comment — only ever present
                            // on an initial-connect/post-resize snapshot.
                            if (msg.data.value.altScreenActive !== undefined) {
                                onAltScreenActiveHint?.(msg.data.value.altScreenActive);
                            }
                            // Handle raw output
                            const decodedData = msg.data.value.data;
                            if (decodedData.length === 0) {
                                continue;
                            }
                            const text = textDecoderRef.current.decode(decodedData, {
                                stream: true
                            });
                            // Record message if recording is active
                            metrics.recordMessage({
                                timestamp: Date.now(),
                                type: 'raw',
                                data: decodedData,
                                decoded: text
                            });
                            if (false) {}
                            // Use callback if provided, otherwise batch via RAF
                            if (onOutput) {
                                onOutput(text, msg.data.value.resyncId);
                            } else {
                                metrics.scheduleOutputUpdate(text);
                            }
                            // First raw output → terminal is stable (not resizing)
                            setTerminalState((prev)=>prev === 'LOADING' || prev === 'CONNECTING' ? 'STABLE' : prev);
                        } else if (msg.data.case === "scrollbackResponse") {
                            // Use a per-response decoder so chunks within one scrollbackResponse are streamed
                            // correctly, but separate responses don't share stateful decoder state.
                            const responseDecoder = new TextDecoder();
                            const chunks = [];
                            for (const chunk of msg.data.value.chunks){
                                const text = responseDecoder.decode(chunk.data, {
                                    stream: true
                                });
                                chunks.push(text);
                            }
                            // Flush any trailing bytes buffered by the streaming decoder.
                            const trailing = responseDecoder.decode();
                            if (trailing) chunks.push(trailing);
                            const scrollbackText = chunks.join("");
                            const metadata = {
                                hasMore: msg.data.value.hasMore,
                                oldestSequence: Number(msg.data.value.oldestSequence),
                                newestSequence: Number(msg.data.value.newestSequence),
                                totalLines: Number(msg.data.value.totalLines)
                            };
                            console.log(`[useTerminalStream] Scrollback metadata:`, metadata);
                            if (onScrollbackReceived) {
                                onScrollbackReceived(scrollbackText, metadata);
                            }
                            // Scrollback response received — return to STABLE
                            setTerminalState((prev)=>prev === 'FETCHING_SCROLLBACK' ? 'STABLE' : prev);
                        } else if (msg.data.case === "appScrollbackResponse") {
                            // Story 1.4.1 — a captured redraw from Instance.ForwardScroll, sent
                            // instead of scrollbackResponse when AppScrollGate passes. Decoded
                            // and dispatched through its own callback so it can never be
                            // confused with either the raw output or tmux-native scrollback path.
                            const resp = msg.data.value;
                            const text = resp.content.length > 0 ? new TextDecoder().decode(resp.content) : "";
                            onAppScrollback?.(text, resp.outcome, resp.program, resp.forwardId, resp.blockedReason);
                            setTerminalState((prev)=>prev === 'FETCHING_SCROLLBACK' ? 'STABLE' : prev);
                        } else if (msg.data.case === "error") {
                            const err = new Error(msg.data.value.message);
                            setError(err);
                            onError?.(err);
                            // design/ux.md Surface 2 — HUB_START_FAILED means the hub-owned
                            // path failed to start AND its server-side legacy fallback also
                            // failed (server/services/connectrpc_websocket.go's streamViaHub),
                            // so this connection has no working path at all. Reconnecting
                            // would hit the same failure, so skip the usual 5-attempt
                            // backoff-exhaustion path and surface TerminalOutput.tsx's
                            // existing hardFailedBanner immediately — the same treatment
                            // already given to non-retriable ws-close-codes below.
                            if (msg.data.value.code === "HUB_START_FAILED") {
                                shouldReconnectRef.current = false;
                                isHardFailedRef.current = true;
                                setIsHardFailed(true);
                                console.warn(`[reconnect] stream=terminal trigger=hub-start-failed, giving up`);
                            }
                        }
                    }
                } catch (err) {
                    // Task 2.2.3 — a superseded generation's error/teardown must not
                    // stomp the newer generation's state (shared backoff/hard-fail
                    // refs included — a stale, aborted generation's close should not
                    // affect the currently-live generation's reconnect fate).
                    if (myGeneration === connectionGenerationRef.current) {
                        // isWorktreeMissingError is checked here (terminal-stream-specific),
                        // not folded into the shared isNonRetriableConnectError — see that
                        // function's doc comment for why.
                        if ((0,backoff/* isNonRetriableConnectError */.Qi)(err) || (0,backoff/* isWorktreeMissingError */.GN)(err)) {
                            shouldReconnectRef.current = false;
                            isHardFailedRef.current = true;
                            setIsHardFailed(true);
                            const code = err instanceof connect_error/* ConnectError */.T ? err.code : "unknown";
                            console.warn(`[reconnect] stream=terminal non-retriable error code=${code}, giving up`);
                        }
                        // A connect-timeout abort is our own deliberate fast-retry optimization, not
                        // a real failure — don't surface it via onError/setError, or callers that
                        // count onError calls toward a user-visible "connection failed" UI (e.g.
                        // TerminalOutput.tsx's connectionAttempts banner) would show churn/false
                        // "Terminal unavailable" states for what's meant to be an invisible retry.
                        // The internal backoff/reconnect scheduling below is unaffected either way.
                        if (!connectTimeoutAbortedRef.current) {
                            handleError(err);
                        }
                    }
                } finally{
                    if (myGeneration === connectionGenerationRef.current) {
                        // A superseded generation is a deliberate reconnect; reaching here means this stream ended on its own.
                        console.warn(`[stream-disconnect] session=${sessionId} shell=${shellId ?? ""} livedMs=${Date.now() - connectStartedAt} ` + `foreground=${foregroundRef.current} visibility=${document.visibilityState} willReconnect=${shouldReconnectRef.current} ` + `hardFailed=${isHardFailedRef.current} connectTimeoutAbort=${connectTimeoutAbortedRef.current}`);
                        isConnectedRef.current = false; // sync ref before state setter to prevent reconnect guard race
                        isConnectingRef.current = false;
                        setIsConnected(false);
                        setTerminalState('DISCONNECTED');
                        // A stale count from the just-closed connection must not linger
                        // and imply this session still has extra viewers attached.
                        setConnectionCount(undefined);
                        // Reset decoders so stale {stream:true} buffered state from a server-closed
                        // connection does not corrupt the next connect() call.
                        textDecoderRef.current = new TextDecoder();
                        scrollbackDecoderRef.current = new TextDecoder();
                        // Task 2.2.3 — guarded by the same myGeneration check as the rest of this
                        // block: an already-superseded generation's teardown must not clear the
                        // CURRENT generation's pending connect-timeout timer (connectTimeoutRef is
                        // shared across the hook's lifetime, not per-generation).
                        clearConnectTimeout();
                        if (process.env.NEXT_PUBLIC_RECONNECT_V2 === "true" && shouldReconnectRef.current && !isDisconnectingRef.current) {
                            if (reconnectTimerRef.current) {
                                clearTimeout(reconnectTimerRef.current);
                                reconnectTimerRef.current = null;
                            }
                            if (connectTimeoutAbortedRef.current) {
                                // A connect-timeout abort is our own deliberate fast-retry optimization
                                // (see the catch block above), not a real stream failure — it must not
                                // consume the 5-attempt hard-fail budget or escalate the backoff delay,
                                // or a connection whose genuine first-message latency exceeds the
                                // connect-timeout (e.g. large scrollback replay over a high-RTT VPN link)
                                // would eventually hard-fail instead of getting the unlimited patience it
                                // had before the per-attempt connect-timeout existed. Retry immediately —
                                // each cycle is already bounded by the connect-timeout itself, so this
                                // can't thundering-herd the way an unthrottled real-failure retry could.
                                console.info(`[reconnect] stream=terminal trigger=connect-timeout attempt=${terminalBackoffRef.current.attempt} delay=0ms (excluded from backoff)`);
                                reconnectTimerRef.current = setTimeout(()=>{
                                    reconnectTimerRef.current = null;
                                    if (shouldReconnectRef.current && !isDisconnectingRef.current) {
                                        connectRef.current?.(undefined, undefined, {
                                            isAutoRetry: true
                                        });
                                    }
                                }, 0);
                            } else if (terminalBackoffRef.current.attempt >= 5) {
                                shouldReconnectRef.current = false;
                                isHardFailedRef.current = true;
                                setIsHardFailed(true);
                            } else {
                                const delay = terminalBackoffRef.current.next();
                                console.info(`[reconnect] stream=terminal trigger=close attempt=${terminalBackoffRef.current.attempt} delay=${delay}ms`);
                                reconnectTimerRef.current = setTimeout(()=>{
                                    reconnectTimerRef.current = null;
                                    if (shouldReconnectRef.current && !isDisconnectingRef.current) {
                                        connectRef.current?.(undefined, undefined, {
                                            isAutoRetry: true
                                        });
                                    }
                                }, delay);
                            }
                        }
                    }
                }
            })();
        } catch (err) {
            // Covers a synchronous throw before the async message loop starts (e.g. proto
            // schema validation, MessageQueue construction) — the connect-timeout scheduled
            // above must not outlive this now-dead attempt.
            isConnectingRef.current = false;
            clearConnectTimeout();
            handleError(err);
            setIsConnected(false);
        }
    }, [
        sessionId,
        shellId,
        onShellStatusChange,
        getTerminal,
        onError,
        onScrollbackReceived,
        onOutput,
        onAppScrollback,
        onAltScreenActiveHint,
        flowControl,
        metrics,
        handleError,
        initialCols,
        initialRows,
        onInputDropped,
        clearConnectTimeout
    ]);
    // Keep connectRef in sync so visibility/online listeners always call the current closure
    connectRef.current = connect;
    // ---- Disconnect ----
    // Use stable method reference to avoid disconnect being recreated on every render.
    // flowControl returns a new object literal each render, but getIsResyncingRef is a
    // stable useCallback(() => isResyncingRef, []) so depending on it keeps disconnect stable.
    const getIsResyncingRef = flowControl.getIsResyncingRef;
    const disconnect = (0,react.useCallback)(async ()=>{
        shouldReconnectRef.current = false;
        if (reconnectTimerRef.current) {
            clearTimeout(reconnectTimerRef.current);
            reconnectTimerRef.current = null;
        }
        clearConnectTimeout();
        const isResyncingRef = getIsResyncingRef();
        if (isDisconnectingRef.current || isResyncingRef.current) {
            if (isResyncingRef.current) {
                console.log("[useTerminalStream] Delaying disconnect - resync in progress");
                setTimeout(()=>disconnect(), 500);
            }
            return;
        }
        isDisconnectingRef.current = true;
        // Captured so the delayed callback below can tell whether a newer connect()
        // has since taken over before it mutates shared abortControllerRef/isConnected
        // state — otherwise a stale disconnect() racing a fresh connect() can abort
        // or clobber the newer generation's connection (see connection-generation
        // guard on the read side in connect(), Story 2.2).
        const myGeneration = connectionGenerationRef.current;
        if (messageQueueRef.current) {
            const dropped = messageQueueRef.current.close();
            if (dropped > 0) {
                onInputDropped?.(dropped);
            }
            messageQueueRef.current = null;
        }
        await new Promise((resolve)=>{
            const timeout = setTimeout(()=>{
                if (myGeneration === connectionGenerationRef.current && abortControllerRef.current) {
                    console.debug("[useTerminalStream] Timeout waiting for graceful close, forcing abort");
                    abortControllerRef.current.abort();
                    abortControllerRef.current = null;
                }
                resolve();
            }, 1000);
            if (!isConnectedRef.current) {
                clearTimeout(timeout);
                resolve();
                return;
            }
        });
        if (myGeneration === connectionGenerationRef.current) {
            setIsConnected(false);
        }
        isDisconnectingRef.current = false;
        textDecoderRef.current = new TextDecoder();
        scrollbackDecoderRef.current = new TextDecoder();
    }, [
        getIsResyncingRef,
        onInputDropped,
        clearConnectTimeout
    ]);
    // ---- Auto-connect / cleanup ----
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (autoConnect) {
            connect();
        }
        return ()=>{
            shouldReconnectRef.current = false;
            if (reconnectTimerRef.current) {
                clearTimeout(reconnectTimerRef.current);
                reconnectTimerRef.current = null;
            }
            clearConnectTimeout();
            metrics.flushOutputBuffer();
            disconnect();
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        sessionId,
        autoConnect
    ]);
    // Story 3.1.3 — reconnect on tab visibility or network online event
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true") return;
        const handleVisibilityOrOnline = (ev)=>{
            if (document.visibilityState !== "visible" && ev.type !== "online") return;
            if (terminalDebounceTimerRef.current) clearTimeout(terminalDebounceTimerRef.current);
            terminalDebounceTimerRef.current = setTimeout(()=>{
                terminalDebounceTimerRef.current = null;
                if (shouldReconnectRef.current && !isConnectedRef.current && !isDisconnectingRef.current) {
                    terminalBackoffRef.current.reset();
                    console.info("[reconnect] stream=terminal trigger=visibility delay=0ms");
                    connectRef.current();
                }
            }, 200);
        };
        document.addEventListener("visibilitychange", handleVisibilityOrOnline);
        window.addEventListener("online", handleVisibilityOrOnline);
        return ()=>{
            if (terminalDebounceTimerRef.current) clearTimeout(terminalDebounceTimerRef.current);
            document.removeEventListener("visibilitychange", handleVisibilityOrOnline);
            window.removeEventListener("online", handleVisibilityOrOnline);
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
    // Story 3.1.2 — manual reconnect after hard failure
    const handleManualReconnect = (0,react.useCallback)(()=>{
        isHardFailedRef.current = false;
        setIsHardFailed(false);
        shouldReconnectRef.current = true;
        terminalBackoffRef.current.reset();
        connectRef.current();
    }, []);
    return {
        output: metrics.output,
        isConnected,
        error,
        sendInput: flowControl.sendInput,
        isInputChunking: flowControl.isInputChunking,
        resize: flowControl.resize,
        connect,
        disconnect,
        scrollbackLoaded,
        requestScrollback: flowControl.requestScrollback,
        sendFlowControl: flowControl.sendFlowControl,
        startRecording: metrics.startRecording,
        stopRecording: metrics.stopRecording,
        terminalState,
        isHardFailed,
        handleManualReconnect,
        requestFullResync: flowControl.requestFullResync,
        markResyncComplete: flowControl.markResyncComplete,
        markPaneResponseReceived: flowControl.markPaneResponseReceived,
        connectionCount
    };
}

// EXTERNAL MODULE: ./src/lib/hooks/useAnnounce.ts
var useAnnounce = __webpack_require__(88305);
;// ./src/lib/hooks/useFileDrop.ts

const isFileDrag = (e)=>e.dataTransfer.types.includes("Files");
/**
 * Drag-and-drop of OS files onto an element. Ignores non-file drags (e.g. pane reordering)
 * and, when `enabled` is false, swallows file drags (so the browser doesn't navigate to the
 * dropped file) without showing an overlay or uploading. dragCounterRef balances enter/leave events
 * fired by child elements so the overlay doesn't flicker.
 */ // kibitzer:ignore flag-argument -- `enabled` gates the whole hook (read-only/touch), not a mode switch
function useFileDrop(enabled, onFiles) {
    const [isDragOver, setIsDragOver] = (0,react.useState)(false);
    const dragCounterRef = (0,react.useRef)(0);
    const onDragEnter = (0,react.useCallback)((e)=>{
        if (!isFileDrag(e)) return;
        e.preventDefault();
        if (!enabled) return;
        dragCounterRef.current++;
        if (dragCounterRef.current === 1) setIsDragOver(true);
    }, [
        enabled
    ]);
    const onDragOver = (0,react.useCallback)((e)=>{
        if (!isFileDrag(e)) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = enabled ? "copy" : "none";
    }, [
        enabled
    ]);
    const onDragLeave = (0,react.useCallback)((e)=>{
        if (!isFileDrag(e)) return;
        e.preventDefault();
        if (!enabled) return;
        dragCounterRef.current = Math.max(0, dragCounterRef.current - 1);
        if (dragCounterRef.current === 0) setIsDragOver(false);
    }, [
        enabled
    ]);
    const onDrop = (0,react.useCallback)((e)=>{
        dragCounterRef.current = 0;
        setIsDragOver(false);
        if (!isFileDrag(e)) return;
        e.preventDefault();
        if (!enabled) return;
        const files = Array.from(e.dataTransfer.files);
        if (files.length > 0) onFiles(files);
    }, [
        enabled,
        onFiles
    ]);
    return {
        isDragOver: isDragOver && enabled,
        dropHandlers: {
            onDragEnter,
            onDragOver,
            onDragLeave,
            onDrop
        }
    };
}

// EXTERNAL MODULE: ./src/lib/hooks/usePublishStackTopOffset.ts
var usePublishStackTopOffset = __webpack_require__(49635);
// EXTERNAL MODULE: ./src/lib/hooks/useDebounce.ts
var useDebounce = __webpack_require__(80581);
// EXTERNAL MODULE: ./src/lib/contexts/AnalyticsContext.tsx
var AnalyticsContext = __webpack_require__(6000);
;// ./src/components/sessions/useVisibilityResync.ts




const RESYNC_DEBOUNCE_MS = 300;
const RESYNC_STALL_TIMEOUT_MS = 4000;
// Task 3.1.2.4b — hard ceiling on how long a single resync_id may stay
// outstanding, even while sibling resync traffic keeps proving the
// connection alive and resetting this hook's own 4s stall watchdog. Without
// this, a resync whose own server-side handling silently hangs/fails would
// never surface: every reset from an unrelated ID's response looks
// identical to progress. 2x RESYNC_STALL_TIMEOUT_MS gives sibling traffic
// real room to legitimately postpone the watchdog before treating it as
// evidence of an actual (not just slow) failure.
const RESYNC_STALL_ESCALATION_CEILING_MS = RESYNC_STALL_TIMEOUT_MS * 2;
// Delay before surfacing the existing reconnecting-banner UI for a pending
// resync (Story 2.1.8) — shorter than RESYNC_STALL_TIMEOUT_MS so a slow
// resync gets a visible affordance well before the 4s forced-reconnect path.
const RESYNC_BANNER_DELAY_MS = 2000;
// Epic 2.1 (AC1) — scopes visibility/focus-triggered resync to only the
// instance the user is actually looking at. Flag off preserves pre-project
// behavior (every mounted instance resyncs on every visibility/focus event).
const RESYNC_VISIBILITY_SCOPE_FLAG = 'terminal:resync-visibility-scope';
function useVisibilityResync(params) {
    const { sessionId, isVisible, isConnected, terminalState, connect, disconnect, requestFullResync, markResyncComplete, markPaneResponseReceived, setShowReconnectButton, setShowReconnectBanner, scheduleResync, outstandingResyncIdsRef } = params;
    const resyncVisibilityScopeEnabled = (0,FeatureFlagsContext/* useFeatureFlag */.u)(RESYNC_VISIBILITY_SCOPE_FLAG);
    const resyncStallTimerRef = (0,react.useRef)(null);
    // Distinct ref/timer from resyncStallTimerRef and useDebouncedCallback's own
    // internal timer (pitfalls.md guardrail: never share a ref between timers).
    // Fires 2s into a pending resync to surface the existing reconnecting-banner
    // UI (Story 2.1.8) — separate from the 4s stall watchdog above.
    const resyncBannerTimerRef = (0,react.useRef)(null);
    // No-correlation-ID heuristic: cleared by the *next* output of any kind (see
    // notifyResyncOutputReceived below), not specifically the CurrentPaneRequest
    // response — same imprecise heuristic the pre-existing resize->resync path
    // already relies on (isResyncingRef/waitingForPaneResponseRef). Accepted
    // trade-off, locked in by Task 2.1.3d's test; worst case bounded by the
    // RESYNC_STALL_TIMEOUT_MS watchdog below.
    const pendingResyncCompletionRef = (0,react.useRef)(false);
    // Sibling ref to pendingResyncCompletionRef (Epic 3.1, Task 3.1.1.4c) —
    // kept separate rather than folding the ID into pendingResyncCompletionRef
    // itself, since requestFullResync returns undefined when the correlation-ID
    // flag is off, which would break the boolean pending/re-entrancy-guard
    // semantics above if it were the sole "is pending" indicator. Holds the
    // specific resync_id this hook is currently waiting on, or undefined when
    // there's no pending resync or the flag is off.
    const pendingResyncIdRef = (0,react.useRef)(undefined);
    // Tracks whether the reconnecting-banner was actually shown (i.e. the 2s
    // resyncBannerTimerRef fired) for the *current* pending resync — distinct
    // from pendingResyncCompletionRef so notifyResyncOutputReceived only calls
    // setShowReconnectBanner(false) when there is actually a shown banner to
    // hide (Story 2.1.8 AC: "If the resync completes before 2000ms elapses...
    // setShowReconnectBanner is never called").
    const bannerShownRef = (0,react.useRef)(false);
    // Task 7.1.1.4 (Epic 7.1 observability) — timestamp the current pending
    // resync started, so notifyResyncOutputReceived's success path can log how
    // long it actually took (as a companion metric to the 4s stall-watchdog
    // ceiling above, not a replacement for it).
    const resyncStartTimeRef = (0,react.useRef)(null);
    const { track } = (0,AnalyticsContext/* useAnalytics */.st)();
    const isVisibleRef = (0,react.useRef)(isVisible);
    const resyncVisibilityScopeEnabledRef = (0,react.useRef)(resyncVisibilityScopeEnabled);
    const isConnectedRef = (0,react.useRef)(isConnected);
    const terminalStateRef = (0,react.useRef)(terminalState);
    const connectRef = (0,react.useRef)(connect);
    const disconnectRef = (0,react.useRef)(disconnect);
    const requestFullResyncRef = (0,react.useRef)(requestFullResync);
    const markResyncCompleteRef = (0,react.useRef)(markResyncComplete);
    const markPaneResponseReceivedRef = (0,react.useRef)(markPaneResponseReceived);
    const setShowReconnectButtonRef = (0,react.useRef)(setShowReconnectButton);
    const setShowReconnectBannerRef = (0,react.useRef)(setShowReconnectBanner);
    const scheduleResyncRef = (0,react.useRef)(scheduleResync);
    const trackRef = (0,react.useRef)(track);
    // Mirrors the *latest* sessionId on every render, independent of which
    // sessionId a given `handleVisibilityOrFocusResyncInner` closure was created
    // for. Lets a stale, already-scheduled debounced/watchdog callback detect
    // mid-flight that the session has since switched and abort instead of
    // firing against the new session's connect()/disconnect() (2nd-review
    // architecture/adversarial finding: session-switch cleanup, Story 2.1.6,
    // only tears down state for a resync/watchdog that had already started —
    // it doesn't cancel a debounce timer that's still pending when the switch
    // happens, since useDebouncedCallback exposes no cancel handle).
    const sessionIdRef = (0,react.useRef)(sessionId);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        isVisibleRef.current = isVisible;
        resyncVisibilityScopeEnabledRef.current = resyncVisibilityScopeEnabled;
        isConnectedRef.current = isConnected;
        terminalStateRef.current = terminalState;
        connectRef.current = connect;
        disconnectRef.current = disconnect;
        requestFullResyncRef.current = requestFullResync;
        markResyncCompleteRef.current = markResyncComplete;
        markPaneResponseReceivedRef.current = markPaneResponseReceived;
        setShowReconnectButtonRef.current = setShowReconnectButton;
        setShowReconnectBannerRef.current = setShowReconnectBanner;
        scheduleResyncRef.current = scheduleResync;
        trackRef.current = track;
        sessionIdRef.current = sessionId;
    });
    const clearStallTimer = (0,react.useCallback)(()=>{
        if (resyncStallTimerRef.current) {
            clearTimeout(resyncStallTimerRef.current);
            resyncStallTimerRef.current = null;
        }
    }, []);
    const clearBannerTimer = (0,react.useCallback)(()=>{
        if (resyncBannerTimerRef.current) {
            clearTimeout(resyncBannerTimerRef.current);
            resyncBannerTimerRef.current = null;
        }
    }, []);
    // Shared terminal-recovery path (Epic 3.1, Tasks 3.1.2.2/3.1.2.4b) for both
    // ways a pending resync can be declared lost: the ordinary 4s stall
    // watchdog firing with no sibling traffic to reset it, or resetStallWatchdog
    // below discovering this hook's own resync_id has been outstanding past the
    // 2x escalation ceiling despite repeated resets. Identical recovery
    // (disconnect+reconnect) either way — only the log/analytics label differs,
    // so a stall dashboard can distinguish "no reset ever happened" fires from
    // "resets kept happening but the real response never came" ones.
    const forceResyncRecovery = (0,react.useCallback)((reason)=>{
        if (!pendingResyncCompletionRef.current) return;
        const stalledResyncId = pendingResyncIdRef.current;
        // Actual wall-clock time since this resync's request was sent, not just the
        // nominal watchdog constant — an escalation-triggered fire can have been
        // outstanding far longer than RESYNC_STALL_ESCALATION_CEILING_MS if sibling
        // traffic kept resetting the per-fire timer (see this function's own doc
        // comment). Falls back to the nominal constant if the start time was
        // somehow never recorded, so the log line never shows an undefined/NaN
        // duration.
        const actualElapsedMs = resyncStartTimeRef.current !== null ? Date.now() - resyncStartTimeRef.current : undefined;
        pendingResyncCompletionRef.current = false;
        pendingResyncIdRef.current = undefined;
        bannerShownRef.current = false;
        resyncStartTimeRef.current = null;
        markResyncCompleteRef.current();
        markPaneResponseReceivedRef.current();
        clearStallTimer();
        clearBannerTimer();
        const elapsedMs = reason === 'escalation' ? RESYNC_STALL_ESCALATION_CEILING_MS : RESYNC_STALL_TIMEOUT_MS;
        const label = reason === 'escalation' ? 'escalation ceiling' : 'stall watchdog';
        // resyncId is included so a future occurrence can be grepped directly out of
        // the server's structured log (matches the resync_id field logged wherever
        // EchoResyncID/terminal:resync-correlation-id echoes it back) without first
        // having to correlate on session name + rough timestamp, which is how the
        // 2026-08-25 investigation into this exact symptom (see
        // session/tmux/tmux.go's resyncFastLaneTimeout doc comment) had to be done.
        console.warn(`[resync] sessionId=${sessionIdRef.current} resyncId=${stalledResyncId ?? '(none)'} ${label} fired after ${elapsedMs}ms nominal (${actualElapsedMs ?? 'unknown'}ms actual), forcing disconnect+reconnect`);
        // Task 7.1.1.5 (Epic 7.1 observability) — structured analytics event
        // alongside the console.warn above, so stall-watchdog/escalation fires
        // are queryable (e.g. correlated with visibility_state) rather than only
        // discoverable by grepping console output.
        trackRef.current({
            name: reason === 'escalation' ? 'resync_stall_escalation_fired' : 'resync_stall_watchdog_fired',
            category: 'performance',
            durationMs: elapsedMs,
            labels: {
                resync_id: stalledResyncId ?? '',
                visibility_state: document.visibilityState
            }
        });
        disconnectRef.current().then(()=>connectRef.current());
    }, [
        clearStallTimer,
        clearBannerTimer
    ]);
    // Extracted (Epic 3.1, Task 3.1.2.2) so both the initial resync trigger and
    // the shared resetStallWatchdog() below can (re)arm the same 4s watchdog
    // without duplicating its body.
    const armStallTimer = (0,react.useCallback)(()=>{
        clearStallTimer();
        resyncStallTimerRef.current = setTimeout(()=>{
            resyncStallTimerRef.current = null;
            forceResyncRecovery('watchdog');
        }, RESYNC_STALL_TIMEOUT_MS);
    }, [
        clearStallTimer,
        forceResyncRecovery
    ]);
    const handleVisibilityOrFocusResyncInner = (0,react.useCallback)((preempt = false)=>{
        if (document.visibilityState !== 'visible') return;
        // Epic 2.1 (AC1) — with the flag on, only the instance actually in the
        // foreground resyncs; a backgrounded instance is a no-op here and will
        // resync on its own once the user switches to it (its own isVisible flips
        // true and fires this same handler). Flag off preserves pre-project
        // behavior: every mounted instance resyncs on every visibility/focus event
        // regardless of isVisible.
        if (resyncVisibilityScopeEnabledRef.current && !isVisibleRef.current) return;
        // Abort if the session changed since this debounced call was scheduled —
        // e.g. visibilitychange fires while viewing session A, the debounce timer
        // is armed, then the user switches to session B before the 300ms elapses.
        // `sessionId` here is this closure's frozen value (a real useCallback dep);
        // `sessionIdRef.current` is the latest live value. A mismatch means this is
        // a stale callback that must not act on the new session's connect/disconnect
        // (2nd-review finding: Story 2.1.6's cleanup only handles a resync/watchdog
        // that already started, not one still pending in the debounce window).
        if (sessionId !== sessionIdRef.current) return;
        if (isConnectedRef.current) {
            // Rapid-flap re-entrancy guard — see Story 2.1.3.
            if (pendingResyncCompletionRef.current) return;
            pendingResyncCompletionRef.current = true;
            // Epic 6.1 (terminal:resync-stagger) — `fire` is exactly today's
            // pre-Epic-6.1 synchronous resync logic, unchanged. When no scheduler
            // is wired (flag off, or no SessionDetailView-level stagger queue),
            // it's invoked immediately below, preserving byte-for-byte behavior.
            const fire = ()=>{
                try {
                    console.info(`[resync] sessionId=${sessionId} trigger=visibility-or-focus delay=0ms`);
                    resyncStartTimeRef.current = Date.now();
                    pendingResyncIdRef.current = requestFullResyncRef.current(true, true);
                } catch (err) {
                    console.warn(`[resync] sessionId=${sessionId} requestFullResync threw synchronously`, err);
                } finally{
                    // Arm unconditionally — even on a synchronous throw — so the watchdog
                    // remains the single recovery path. See Story 2.1.3.
                    armStallTimer();
                    // Surface the existing reconnecting-banner UI once a resync has been
                    // pending ≥2s — see Story 2.1.8. Independent of, and shorter than, the
                    // 4s stall watchdog above; reuses `TerminalOutput.tsx`'s already-shipped
                    // banner rather than adding a new resync-specific indicator (Pattern
                    // Decisions: "Transient-state UI during the 0-4s resync/watchdog window").
                    clearBannerTimer();
                    bannerShownRef.current = false;
                    resyncBannerTimerRef.current = setTimeout(()=>{
                        resyncBannerTimerRef.current = null;
                        if (pendingResyncCompletionRef.current) {
                            bannerShownRef.current = true;
                            setShowReconnectBannerRef.current?.(true);
                        }
                    }, RESYNC_BANNER_DELAY_MS);
                }
            };
            if (scheduleResyncRef.current) {
                scheduleResyncRef.current(fire, {
                    preempt
                });
            } else {
                fire();
            }
        } else {
            // Don't take the disconnected fallback mid-handshake. See Story 2.1.2.
            if (terminalStateRef.current === 'CONNECTING' || terminalStateRef.current === 'LOADING') return;
            console.info(`[resync] sessionId=${sessionId} trigger=visibility-or-focus fallback=connect`);
            connectRef.current();
            setShowReconnectButtonRef.current(true);
        }
    }, [
        sessionId,
        clearBannerTimer,
        armStallTimer
    ]);
    // Zero-arg wrapper: `debouncedResync` is registered directly as a DOM event
    // listener below, so useDebouncedCallback forwards whatever argument the
    // browser passes it (the Event object) straight through to the wrapped
    // callback. Without this indirection that Event would land in
    // handleVisibilityOrFocusResyncInner's `preempt` parameter (added for Task
    // 6.1.1.3) and every ordinary visibility/focus resync would be
    // misidentified as a preempting one.
    const handleVisibilityOrFocusResync = (0,react.useCallback)(()=>{
        handleVisibilityOrFocusResyncInner(false);
    }, [
        handleVisibilityOrFocusResyncInner
    ]);
    // Epic 1.2's useDebouncedCallback (ref-backed timer, memoized return) IS the
    // debounce mechanism here — not a hand-rolled setTimeout/ref pair — making
    // Epic 1.2's "first real consumer" framing true of the code (resolves
    // architecture review Concern "Epic 1.2 vs Task 2.1.1d mismatch").
    const debouncedResync = (0,useDebounce/* useDebouncedCallback */.Y)(handleVisibilityOrFocusResync, RESYNC_DEBOUNCE_MS);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        document.addEventListener('visibilitychange', debouncedResync);
        window.addEventListener('focus', debouncedResync);
        return ()=>{
            document.removeEventListener('visibilitychange', debouncedResync);
            window.removeEventListener('focus', debouncedResync);
        };
    }, [
        debouncedResync
    ]);
    // Task 6.1.1.3 ("newly-focused preempts queued") — today, nothing reacts to
    // an `isVisible` prop transition at all; only the global visibilitychange/
    // focus listeners above trigger a resync. Gated strictly on
    // `scheduleResync` being provided (i.e. `terminal:resync-stagger` is on and
    // SessionDetailView has wired its stagger queue in) so this new trigger
    // point is entirely absent when the flag is off, preserving AC7 flag-off
    // byte-for-byte parity — without a scheduler there is nothing for
    // "preempt" to mean, since there's no queue to jump ahead of.
    const wasVisibleRef = (0,react.useRef)(isVisible);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const becameVisible = isVisible && !wasVisibleRef.current;
        wasVisibleRef.current = isVisible;
        if (becameVisible && scheduleResyncRef.current) {
            handleVisibilityOrFocusResyncInner(true);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        isVisible
    ]);
    // sessionId-keyed cleanup: a watchdog/resync armed for the previous session
    // must never fire against the next one's connect()/disconnect() (adversarial
    // review Blocker 1 / research features.md race #4).
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        return ()=>{
            clearStallTimer();
            clearBannerTimer();
            pendingResyncCompletionRef.current = false;
            pendingResyncIdRef.current = undefined;
            bannerShownRef.current = false;
            resyncStartTimeRef.current = null;
            setShowReconnectBannerRef.current?.(false);
            markResyncCompleteRef.current();
            markPaneResponseReceivedRef.current();
        };
    }, [
        sessionId,
        clearStallTimer,
        clearBannerTimer
    ]);
    const notifyResyncOutputReceived = (0,react.useCallback)((resyncId)=>{
        if (!pendingResyncCompletionRef.current) return;
        // Only clear pending state when the IDs match, or when either side lacks
        // an ID (correlation-ID flag off, or this output isn't a resync reply at
        // all) — preserves the pre-Epic-3.1 any-output-clears heuristic in that
        // case while preventing a *different* resync's stray output from
        // clearing this one's pending state when IDs are present and mismatch
        // (Epic 3.1, Task 3.1.1.4c / validation.md
        // notifyResyncOutputReceived_should_NotClearPendingResync_When_ResyncIdMismatch).
        if (resyncId && pendingResyncIdRef.current && resyncId !== pendingResyncIdRef.current) {
            // Task 7.1.1.3 (Epic 7.1 observability) — client-side correlation-ID
            // mismatch, mirrored by the server-side log in handleCurrentPaneRequest
            // (connectrpc_websocket.go) for the case where the mismatch instead
            // stems from the server never having echoed an ID at all.
            console.debug(`[resync] sessionId=${sessionId} resync_id mismatch: expected=${pendingResyncIdRef.current} received=${resyncId}`);
            return;
        }
        const resyncDurationMs = resyncStartTimeRef.current !== null ? Date.now() - resyncStartTimeRef.current : undefined;
        resyncStartTimeRef.current = null;
        pendingResyncCompletionRef.current = false;
        pendingResyncIdRef.current = undefined;
        clearStallTimer();
        clearBannerTimer();
        // The success path never flips isConnected false, so the pre-existing
        // isConnected-driven banner effect (TerminalOutput.tsx:736-761) won't
        // hide a banner shown by the 2s timer above — hide it explicitly here,
        // but only if the banner was actually shown (Story 2.1.8 AC: a resync
        // that completes before the 2s banner delay must never touch
        // setShowReconnectBanner at all).
        if (bannerShownRef.current) {
            setShowReconnectBannerRef.current?.(false);
        }
        bannerShownRef.current = false;
        markResyncCompleteRef.current();
        markPaneResponseReceivedRef.current();
        console.log(`[resync] sessionId=${sessionId} pane response received, resync complete`);
        // Task 7.1.1.4 (Epic 7.1 observability) — success-path duration, alongside
        // the existing stall-watchdog warning (armStallTimer above), so a normal
        // resync's actual latency is visible without waiting for one to stall.
        if (resyncDurationMs !== undefined) {
            console.debug(`[resync] sessionId=${sessionId} resync completed in ${resyncDurationMs}ms`);
        }
    }, [
        sessionId,
        clearStallTimer,
        clearBannerTimer
    ]);
    // Epic 3.1, Task 3.1.2.1/3.1.2.2/3.1.2.4b — lets TerminalOutput.tsx reset
    // this hook's stall watchdog when a matching resync_id arrives via the
    // sibling useTerminalFlowControl/useTerminalStream resize-resync path,
    // without merging the two hooks. No-op if no resync is pending here.
    //
    // Task 3.1.2.4b: a reset must not be able to mask a genuinely-stalled
    // resync forever just because sibling traffic keeps proving the
    // connection alive. Before re-arming, check this hook's own pending
    // resync_id's recorded start time in the shared outstandingResyncIdsRef
    // map (Task 3.1.2.4a) — if it's been outstanding at or past the 2x
    // escalation ceiling, escalate (force disconnect+reconnect) instead of
    // resetting again. `outstandingResyncIdsRef` is optional (pre-3.1.2.4
    // callers/tests) — resetStallWatchdog always just re-arms when it's absent
    // or the ID's start time isn't recorded, preserving prior behavior.
    const resetStallWatchdog = (0,react.useCallback)(()=>{
        if (!pendingResyncCompletionRef.current) return;
        const ownResyncId = pendingResyncIdRef.current;
        const startedAt = ownResyncId ? outstandingResyncIdsRef?.current.get(ownResyncId) : undefined;
        if (startedAt !== undefined && Date.now() - startedAt >= RESYNC_STALL_ESCALATION_CEILING_MS) {
            forceResyncRecovery('escalation');
            return;
        }
        armStallTimer();
    }, [
        armStallTimer,
        forceResyncRecovery,
        outstandingResyncIdsRef
    ]);
    return {
        notifyResyncOutputReceived,
        resetStallWatchdog
    };
}

// EXTERNAL MODULE: ./src/lib/api/transport.ts
var transport = __webpack_require__(73396);
;// ./src/lib/hooks/useBrowserLogStream.ts
/* __next_internal_client_entry_do_not_use__ useBrowserLogStream auto */ 




const MAX_BUFFER = 50;
const MAX_MSG_LEN = 200;
const FLUSH_INTERVAL_MS = 5000;
// NOTE: If TerminalOutput is ever rendered more than once simultaneously
// (session pool scenario), multiple instances of this hook will each patch
// console.*. Since each install captures the then-current console.log as
// its originals.log, the interceptors chain automatically. Cleanup unwinds
// in reverse React effect order. A module-level reference counter should be
// added if multi-mount becomes a real scenario.
function useBrowserLogStream(options) {
    const enabledRef = (0,react.useRef)(options.enabled);
    const sessionRef = (0,react.useRef)(options.sessionId);
    const baseUrlRef = (0,react.useRef)(options.baseUrl);
    // Keep refs current without triggering re-effects
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        enabledRef.current = options.enabled;
    }, [
        options.enabled
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        sessionRef.current = options.sessionId;
    }, [
        options.sessionId
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        baseUrlRef.current = options.baseUrl;
    }, [
        options.baseUrl
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        // SSR guard
        if (true) return;
        // Always install interceptors — console.error and uncaught errors stream to the
        // server unconditionally (see enqueue()) as a crash-diagnostics safety net;
        // log/warn/debug are gated behind options.enabled, since those are the noisy,
        // opt-in-only levels.
        const apiBase = baseUrlRef.current ?? (0,config/* getApiBaseUrl */.h)();
        const client = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,transport/* getConnectTransport */.qW)());
        const buffer = [];
        let flushTimer = null;
        let intercepting = false;
        // Capture originals BEFORE patching
        const originals = {
            log: console.log.bind(console),
            warn: console.warn.bind(console),
            error: console.error.bind(console),
            debug: console.debug.bind(console)
        };
        function truncate(s, max) {
            if (s.length <= max) return s;
            return s.slice(0, max) + "…";
        }
        function argsToMessage(args) {
            try {
                return args.slice(0, 5).map((a)=>typeof a === "object" ? JSON.stringify(a) : String(a)).join(" ");
            } catch  {
                return args.slice(0, 5).map(String).join(" ");
            }
        }
        function enqueue(level, args) {
            // Only "debug" is opt-in (noisy, per-frame tracing); log/warn/error
            // stream unconditionally, matching the server's info-and-up default.
            if (level === "debug" && !enabledRef.current) return;
            if (intercepting) return; // reentrancy guard
            intercepting = true;
            try {
                const entry = {
                    level,
                    message: truncate(argsToMessage(args), MAX_MSG_LEN),
                    timestamp: new Date().toISOString(),
                    url: window.location.href,
                    userAgent: navigator.userAgent,
                    sessionId: sessionRef.current
                };
                buffer.push(entry);
                if (buffer.length >= MAX_BUFFER) {
                    flush();
                } else if (!flushTimer) {
                    flushTimer = setTimeout(flush, FLUSH_INTERVAL_MS);
                }
            } finally{
                intercepting = false;
            }
        }
        function flush() {
            if (flushTimer) {
                clearTimeout(flushTimer);
                flushTimer = null;
            }
            if (buffer.length === 0) return;
            const entries = buffer.splice(0);
            // Use ConnectRPC client — fire-and-forget; never recurse into console.
            // Deliberately not cancelled: buffered log entries should still ship
            // even if the effect that scheduled this flush re-runs first.
            // abort-signal-exempt
            client.logClientEvents({
                entries: entries.map((e)=>({
                        level: e.level,
                        message: e.message,
                        timestamp: e.timestamp,
                        url: e.url,
                        userAgent: e.userAgent,
                        sessionId: e.sessionId ?? ""
                    }))
            }).catch(()=>{});
        }
        // Install interceptors
        console.log = (...a)=>{
            originals.log(...a);
            enqueue("log", a);
        };
        console.warn = (...a)=>{
            originals.warn(...a);
            enqueue("warn", a);
        };
        console.error = (...a)=>{
            originals.error(...a);
            enqueue("error", a);
        };
        console.debug = (...a)=>{
            originals.debug(...a);
            enqueue("debug", a);
        };
        // window.onerror
        const prevOnError = window.onerror;
        window.onerror = (msg, src, line, col, err)=>{
            originals.error("[onerror]", msg, src, line, col, err?.stack);
            enqueue("error", [
                String(msg),
                `${src}:${line}:${col}`,
                err?.stack ?? ""
            ]);
            return prevOnError?.(msg, src, line, col, err) ?? false;
        };
        // unhandledrejection
        function onUnhandled(e) {
            originals.error("[unhandledrejection]", e.reason);
            enqueue("error", [
                "UnhandledRejection",
                String(e.reason)
            ]);
        }
        window.addEventListener("unhandledrejection", onUnhandled);
        // Page-unload beacon — flush remaining entries synchronously
        function onBeforeUnload() {
            if (buffer.length === 0) return;
            const entries = buffer.splice(0);
            // Use sendBeacon with JSON blob for keepalive semantics on unload.
            // ConnectRPC binary POST is not suitable here; fall back to plain JSON
            // matched by the same route (the server accepts both via HTTP/1.1 fallback).
            const body = JSON.stringify({
                entries: entries.map((e)=>({
                        level: e.level,
                        message: e.message,
                        timestamp: e.timestamp,
                        url: e.url,
                        user_agent: e.userAgent,
                        session_id: e.sessionId ?? ""
                    }))
            });
            const blob = new Blob([
                body
            ], {
                type: "application/json"
            });
            if (!navigator.sendBeacon(apiBase + "/session.v1.SessionService/LogClientEvents", blob)) {
                fetch(apiBase + "/session.v1.SessionService/LogClientEvents", {
                    method: "POST",
                    headers: {
                        "Content-Type": "application/json"
                    },
                    body,
                    keepalive: true
                }).catch(()=>{});
            }
        }
        window.addEventListener("beforeunload", onBeforeUnload);
        // Start periodic flush
        flushTimer = setTimeout(flush, FLUSH_INTERVAL_MS);
        // Cleanup: restore originals, cancel timer, clear buffer
        return ()=>{
            console.log = originals.log;
            console.warn = originals.warn;
            console.error = originals.error;
            console.debug = originals.debug;
            window.onerror = prevOnError;
            window.removeEventListener("unhandledrejection", onUnhandled);
            window.removeEventListener("beforeunload", onBeforeUnload);
            if (flushTimer) {
                clearTimeout(flushTimer);
                flushTimer = null;
            }
            // Discard pending buffer on cleanup — the beforeunload beacon handles
            // data preservation on actual page unload.
            buffer.length = 0;
        };
    // Install once per mount — options.enabled is read live via enabledRef inside
    // enqueue(), so toggling it doesn't need to reinstall the console interceptors.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);
}

// EXTERNAL MODULE: ./src/lib/hooks/useHandedness.ts
var useHandedness = __webpack_require__(87249);
;// ./src/lib/hooks/useSplitContainerSize.ts
/* __next_internal_client_entry_do_not_use__ useSplitContainerSize auto */ 
/**
 * useSplitContainerSize — tracks the rendered size of a container element
 * via ResizeObserver. Used by the pane keyboard-nudge resize to compute
 * `containerSizePx` for the NUDGE_RESIZE action.
 */ function useSplitContainerSize(ref) {
    const [size, setSize] = (0,react.useState)({
        width: 0,
        height: 0
    });
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const el = ref.current;
        if (!el) return;
        const ro = new ResizeObserver(([entry])=>{
            const { width, height } = entry.contentRect;
            setSize({
                width,
                height
            });
        });
        ro.observe(el);
        return ()=>ro.disconnect();
    }, [
        ref
    ]);
    return size;
}

// EXTERNAL MODULE: ./src/lib/terminal/TerminalPool.tsx
var TerminalPool = __webpack_require__(56935);
// EXTERNAL MODULE: ./src/lib/terminal/TerminalPool.css.ts
var TerminalPool_css = __webpack_require__(45500);
// EXTERNAL MODULE: ./src/components/ui/LiveRegion.tsx
var LiveRegion = __webpack_require__(20606);
// EXTERNAL MODULE: ./src/lib/notification-policy.ts
var notification_policy = __webpack_require__(30550);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fstyles%2Ftheme.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-2cS2_cOBKA7_MrCPjgGUAO-NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y-Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7-Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT-hCZpMi_vQJ5G11dz-XuYF-u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY-rnak9Lag-uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk-9YPwGvNEkuEhBEszfgD-65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE-EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV-gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3-Je_uvvl7l69Xi3hn3er5Wq7jj29-W2rtu-2u3R3cb8o1EVcr2_TxSQp1K-3i-XN69j_if_-fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh-1bVVOh0q4zNpKZVpsyktlU2U-tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD-56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK-ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK-FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp-TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj-hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ-kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy-0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g-AOAX5mttB5BT22AFjqXwQ6qdpv8RR-TgNVKbncr-_QuVX045dNuMLlMes3Wuh3RuU-8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X-Ix7qfL4ehPqIh2M-zu-QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO-Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR-cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO-WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY-u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192-8OX3X4Cu-8L7GbCr_QO4xksOgYWxz4l-rYHVsC-7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ-CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L-JX7CketZoWGYRPsPkb2KJtDl8bM-N_0MWGyIngMsGrIwPgj1H4XxeTeNc-bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI-OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv-1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ-mRlfTteMK8_2Y6Vlgltx5-jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY-2dOFs8dwclaLx0lmDBEnOS9FnGTDUo-fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3-Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm-2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP-EZIRxEiuRVn-l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd-zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I-iHIF6buh_JfEIMDz5Uidls5OpDBPejL-n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83-m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI-V8uaYPxIVUAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fstyles_2Ftheme_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_2cS2_cOBKA7_MrCPjgGUAO_NRrbzPAALNALjuY84CSKLv96O50t2Mni_3viyp1O1axqp3XboCkLQNJilSRIquK9YliXv1tOhMfVwut_v2TUhcXf5smvbeb97pVZzrC9Y_Z3LTqzEe45nLbqrOyg2sud606ayJcc7lv1VlVwjWXh1adjfgzl5eCvAJ5A9dcXoNcwzWXN4Ke2KrNZRd_1oXa_74Kv8yrdHCrG_1IutxPXYgjefRBqJ9adZY0XHP5CPKQqkT0XLbqbDBDGIj8Coa2jy6SR1kI8muh3RvQ7_Gay2_BBHSlRzeX3wnyJcqD6fu5fIVyNzRkitaC_I0wRRtBvhVMdSfI71t1ZnTX1GYuf4vjPMYU5vIH0FOG0ZN5fIT_hCZpMi_vQJ5G11dz_XuYF_u1TsSlwNfS6L0nE2AMako2WVIA3tY0pjOkaQPu1jWmN2QODPhbl0wdBlKADtenis6mAY_rnak9Lag_uIoxtlDWO3AXXRN_MTVf0ZS0YiNYgonClJtOCk_9YPwGvNEkuEhBEszfgD_65JKnow8OaQc7OOJI5krqLrikDTZYYkLmWurujdQ4eGUa4CIF4Jadh4sULCWbAMccmqrStFfgmWbwaahJwRuM8pWt6ZODb_ouhJIay1YaEvBOE_EiBeCe1vYh0Il6K3UXHMuU0WVrEjjW0NsyG_aHvWVaUxXGNEVZ6FfGUrN8PNSyhSldUXm21runWrpwNfwytaxBCygdDe7WCkNqwZV7bcuBDKn1T24FXgVOpV952l5gKmWdKlu128Tldh03abkjhZUQ5m0tTJBthIBowY19FUJJ_Ntdt2q5WhI97qZVWpn1o7LrR6UPz4HXK01XZXcLtf36UZXrR3UBt81vqH4plEZVUOnCZhUyjXfYvl4_KhPgFkdvMajyqU3_okqLuUPqY09NtBciucXsoY8hktXIYrzqUhzp8I8Y4bymscRCvGpcH0YSMuwV_gdEf1KwQI8qgyOxxEK86rqxGqi13giLnr0VVj17hw84OE97tRTyJgvxylXeBLJw27WQIVkpXtnNlAtVNCxZiFfeuTqbKIhXfTVYOuxu2Sqj9d1WpbilZatWmSCUrVtlhTJ736rz16tl7FfnhTp_nZa3_Je_uvvl7l69Xi3hn3er5Wq7jj29_W2rtu_2u3R3cb8o1EVcr2_TxSQp1K_3i_XN69j_if_fbXcFer8z3S5SuqvP84LtY3L7cU2bRZ0YB5mWsVqj63S60cifNeCsxDh_1bVVOh0q4zNpKZVpsyktlU2U_tcq1yuwbfK562FVpW5hpLpratalfegZnvbtKppmiaTR7Z21yqTN9ez0oEdh9Qqm425G1vlNTF7d9mqkAmvWlVmwkWrKhD_56dXB0Q0DCKymYiQiMBKV0e4SFsCCkLKWmu45vIg5H6lIK_ETAPXsQjXXN4I9RlErBhE5FRClDejbRyJjoMQ0SDGO_yZy0eh_qWQfl0J8W8hyK_FdiG6ew3XXH6LmXDT08B7J8ghtLsuNolMNUT2YLuG0jomorEae_K8xxCRM4GviYi69MmR5wJEdH5wFGCOIWJVu6BJ_yGTHbtutDSHP4qI1WgGQ53QCvkDIiKrCvwt9b7OKCxIJIgvZbp_TJQTDojoQ6FMKAtlTYPoJyEirZhl2Y1gUYiInMshIrLhqZcg5sdGRBaUjiIixzPfNyLSvJ5HxKwWi4gZI_wPENGHArwKnOoII85qfTokcm75bSHRfSIj_hcQMdP3IiGGFwgxmwuIVj7kKxECIsuBgxD5ERBNcjHQ3HyUOPBSsjyIVsYHZ_kdEK3qMo2RvFNCQGTJ8UZY8hAQuTXPTtEqOE0MAgGRy6YQELlQiYDIkuNRQHTGJpoBICDG0PmRPvlOIMcTIM6qnQDxuwVEy_0h6nH0xIcMyvueUoZFeZdFG4dy29FXSB7lZWlJAoSAiD9zeSnIK4ECYA3Tg4m0Pm5XBLjm8g_AOAX5mttB5BT22AFjqXwQ6qdpv8RR_TgNVKbncr_QuVX045dNuMLlMes3Wuh3RuU_8ycbgUDuBPkiIduHEsGDzmDmXYQ_WAIf7wRJnojyLeCfCfI74X_Ix7qfL4ehPqIh2M_zu_QaXI9iIdj32foprFAZ3yOeAh6DC1APBzLsqbohjllD7NMChAPS5OxO_Ih20a5byNL7D_sINoQijIUR8jwWR0WCtkAEwUPnfYNuTt6wZR_cChkLWGFBXVNzdOs93Eje47JIWttKS1uhBButtJEST5pJKdEKOz73MsQCrk77ORO_WuwD1CofWGq_WaQCIXPzVtmwhCKYOBXYkJuRK0VRhSZEEJBdofPe5U1FxinyyqVeSVNCWm_e8iM7TcCQzHQyGz4fPIkPpTVHkHEZwFNokTZcJAUuXA7kSKzCCApwuKc3ZH261WWiI3S7F0KGYC9EpY_u5BUSckEkqKJJvPwiRSZ5RJJ0fs870FS5DI7K2UUdi2kUPaNkJzazeHJaRtbqY2d9OT3rXobNz9fXIyr5e7iOu26TVwstxcAcsUT7z1hHbWMt192_8OX3X4Cu_8L7GbCr_QO4xksOgYWxz4l_rYHVsC_7pImiRksgDHmwcBNH6iWmnidn3YfNX01jLA4AGXN5aUgx91Eayzdb6j3L6sdSTkxRx31SDOcj4JFTiHuJZamtEQ_CPXTQU7mBj8X8cY78iCYpVprAwOLkL2WRM8C0yvr6evGa6HdG0HPrWAAd4L_JX7CketZoWGYRPsPkb2KJtDl8bM_N_0MWGyIngMsGrIwPgj1H4XxeTeNc_bI7yWHmrJbO1QUdV6AxbqOdCPG7eempHd4fNXsXKJ3HIXFXtfZe9UnWISUCHYqrD6Gi7NaLDBytjQBI_OlCIzsKPZCnPnBgZE1rBUWDNmysgfGwbPA2MAXqLRx8Mqqs0NNSfLoLiIXyI3kmAiMbHffC3dY0aUenpmlNeGjgNGE5kVi_OAIAjLmY4rIyI3pEzLSjf8nZJy1eAwa4VPTELj9rZJTpbPNxkrq_DfBxqMhRwJHOj08Oh5XLcIjCW88Pr5gIC8AZG7FTwBpacEeIA0Nm3uAZObxUsgKngCSLodPAJk53QEgs8ZvhBXRSinGHiBd9kZr2mpksj27EtJPu5YeEEJaXeeL6ASQgx6y59hKqe9OauOrAuQmXg9Xcbkozv_1_9t58WGH78MG33GO_FwtJ5w84eQn4KRncLKvu5qeO4TVMdZNncU4kOu6rjicrL12pD4muLrWtD7iZK8j9dlSkO8_Tg00GE_rnUl0PwI_TrU60bdNH4WTnELEydp4urk6CPXx09TBacvsPdpotSH4iZ_mRlfTteMK8_2Y6Vlgltx5_jnItdDujaD_VtB_J8iXQrurvWHQcUCc1KGmE_dGmOiNIJfy1p0gv8fnrXrH4STsIRP9gJP1OPiKGMyjMA57nMxcHnLf5Ps622IET6s75iu9PU7WtEeIk4MvdUaNDr_GMTkceixwedYfpDbA3zpd1z3ltgNOmgY_2dOFs8dwclaLx0lmDBEnOS9FnGTDUo_fR5V1OO0_HgqWkmGtpGGfci8fMzOZPvMq8_3xzUTxOhuSrcT9e5zs2P1HtldvpQJwqRCrSN_AWdGlHljjFXmy0YWxtgjs96ZPPGlcU9gSfiWc5MYUcZKzWMRJeKhAH8tzvRe_Sn3eLQ4nj3_Uyg34t4HIo4FGgkg6wTxEHlctQiQJajxEHjexA0RmgRchkrPdA0Tm_2RpWrk0XWERIrlgiRDJ5QITRDKLIEIkt2pO36tGG2nacoDIbB20UmKBEAm94iGSyfEQIlkXWksP_EZIRxEiuRVn_l6VS3h3UhtfFSL7xfJ9ui3Of8M_z4uPIMaPuuWEhyc8_AQ8DNzZRZtqeuAM8VDH1FOnAE8d6hSJFzlhffiss4ujMYZ80l5Nm30dfTeEu43JBpq3NEJ9iofcwUUTrXXkvl7Qd_zgokm2oQfkRkE_RvLgAj2Xc_TgoivLkbR7LbR7I_iHIF6buh_JfEIMDz5Uidls5OpDBPejL_n_GTMdOHA6kt2h_8dmI3cQ8XRwcVbw4sHF2hd1VVjXHIPD55VYNuQsdWJDxqWmA4tcTDodWPxKBxY5Tz0dWPyiA4vckH7BgcWmKYyG_6_GMMfuntBwVivv14twyEXy04nF04nFecEPe2LxOAGq83_m3a8oOBw0PP99sYnqt9WQ8DTi4VziRyLiYrlLm0Kd_wF_wglD7hThcWT8LBUnhDwh5IsI_V8uaYPxIVUAAA_22_7D = __webpack_require__(13507);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FInputDropBadge.css.ts.vanilla.css%22%2C%22source%22%3A%22QGtleWZyYW1lcyBxZGx6ZW0wIHsKICAwJSB7CiAgICBvcGFjaXR5OiAwOwogICAgdHJhbnNmb3JtOiB0cmFuc2xhdGVYKC01MCUpIHRyYW5zbGF0ZVkoNHB4KTsKICB9CiAgMTAwJSB7CiAgICBvcGFjaXR5OiAxOwogICAgdHJhbnNmb3JtOiB0cmFuc2xhdGVYKC01MCUpIHRyYW5zbGF0ZVkoMCk7CiAgfQp9Ci5xZGx6ZW0xIHsKICBwb3NpdGlvbjogZml4ZWQ7CiAgYm90dG9tOiA4MHB4OwogIGxlZnQ6IDUwJTsKICB0cmFuc2Zvcm06IHRyYW5zbGF0ZVgoLTUwJSk7CiAgei1pbmRleDogMTA4NTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ5KSB2YXIoLS1fMTllejJyejMwKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyenkpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyenopOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzkpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2cpOwogIGJveC1zaGFkb3c6IDAgMnB4IDhweCByZ2JhKDAsMCwwLDAuMyk7CiAgcG9pbnRlci1ldmVudHM6IG5vbmU7CiAgYW5pbWF0aW9uOiBxZGx6ZW0wIDE1MG1zIGVhc2Utb3V0Owp9%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FInputDropBadge_css_ts_vanilla_css_22_2C_22source_22_3A_22QGtleWZyYW1lcyBxZGx6ZW0wIHsKICAwJSB7CiAgICBvcGFjaXR5OiAwOwogICAgdHJhbnNmb3JtOiB0cmFuc2xhdGVYKC01MCUpIHRyYW5zbGF0ZVkoNHB4KTsKICB9CiAgMTAwJSB7CiAgICBvcGFjaXR5OiAxOwogICAgdHJhbnNmb3JtOiB0cmFuc2xhdGVYKC01MCUpIHRyYW5zbGF0ZVkoMCk7CiAgfQp9Ci5xZGx6ZW0xIHsKICBwb3NpdGlvbjogZml4ZWQ7CiAgYm90dG9tOiA4MHB4OwogIGxlZnQ6IDUwJTsKICB0cmFuc2Zvcm06IHRyYW5zbGF0ZVgoLTUwJSk7CiAgei1pbmRleDogMTA4NTsKICBwYWRkaW5nOiB2YXIoLS1fMTllejJyejJ5KSB2YXIoLS1fMTllejJyejMwKTsKICBiYWNrZ3JvdW5kOiB2YXIoLS1fMTllejJyenkpOwogIGNvbG9yOiB2YXIoLS1fMTllejJyenopOwogIGJvcmRlci1yYWRpdXM6IHZhcigtLV8xOWV6MnJ6MzkpOwogIGZvbnQtc2l6ZTogdmFyKC0tXzE5ZXoycnozYik7CiAgZm9udC13ZWlnaHQ6IHZhcigtLV8xOWV6MnJ6M2cpOwogIGJveC1zaGFkb3c6IDAgMnB4IDhweCByZ2JhKDAsMCwwLDAuMyk7CiAgcG9pbnRlci1ldmVudHM6IG5vbmU7CiAgYW5pbWF0aW9uOiBxZGx6ZW0wIDE1MG1zIGVhc2Utb3V0Owp9_22_7D = __webpack_require__(94898);
;// ./src/components/sessions/InputDropBadge.css.ts


var badge = 'qdlzem1';
;// ./src/components/sessions/InputDropBadge.tsx
/* __next_internal_client_entry_do_not_use__ InputDropBadge auto */ 
// +feature: input-drop-badge





function formatBadgeText(count) {
    return `${count} keystroke${count === 1 ? "" : "s"} dropped — reconnecting`;
}
/**
 * Portal-rendered, auto-dismissing badge that visibly and audibly announces
 * a `DropEpisode` (Story 2.3). Modeled on `XtermTerminal.tsx`'s `copiedToast`
 * pattern — see `docs/reference/css-architecture.md` for the vanilla-extract
 * convention used by `InputDropBadge.css.ts`.
 *
 * The nested `LiveRegion` is unconditionally rendered (never gated on the
 * visual badge's visibility) — see design/ux.md §2.4: a stable, always-
 * present live-region DOM node is what assistive technology reliably detects
 * content mutations on.
 */ function InputDropBadge({ count, episodeSeq }) {
    const [visible, setVisible] = (0,react.useState)(false);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        // design/ux.md §3.3 — defensive no-op on count <= 0 (should not happen
        // given MessageQueue.close()'s contract, but a normal user-initiated
        // disconnect with an empty queue should never surface a badge).
        if (count <= 0) {
            return;
        }
        // A new episode always (re)shows the badge and restarts its own
        // auto-dismiss hold from this episode's close time — it does not
        // inherit/extend a prior episode's remaining hold time
        // (design/ux.md §2.3 Case C).
        //
        // No manual dedup ref here: the `[episodeSeq, count]` dependency array
        // already guarantees this effect only re-fires when a genuinely new
        // episode arrives, and this effect's own cleanup (returned below) is
        // what clears the previous episode's timer — including under React
        // StrictMode's dev-only setup→cleanup→setup replay on mount, where a
        // separate cleanup-only effect would otherwise clear the timer armed by
        // setup #1 with no corresponding rearm in setup #2.
        setVisible(true);
        const dismissTimer = setTimeout(()=>{
            setVisible(false);
        }, notification_policy/* DEFAULT_TOAST_MS */.tx);
        return ()=>{
            clearTimeout(dismissTimer);
        };
    }, [
        episodeSeq,
        count
    ]);
    if (typeof document === "undefined") {
        return null;
    }
    const badgeText = count > 0 ? formatBadgeText(count) : "";
    // Task 2.3.2 / adversarial-review.md dedup fix: append an invisible
    // (zero-width-space) nonce derived from episodeSeq so two consecutive
    // episodes with an identical human-readable count still produce a
    // distinct underlying text node — `aria-live` only fires on a genuine DOM
    // text mutation, and React bails on a redundant identical-string update.
    const announcementText = badgeText ? `${badgeText}${"​".repeat(episodeSeq % 2)}` : "";
    return /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
        children: [
            visible && count > 0 && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: badge,
                "data-testid": "input-drop-badge",
                // Decorative only — the LiveRegion below is the sole announcement
                // channel, so this element must not be independently read by AT.
                "aria-hidden": "true",
                children: [
                    "⚠ ",
                    badgeText
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(LiveRegion/* LiveRegion */.L, {
                role: "alert",
                politeness: "assertive",
                message: announcementText
            })
        ]
    }), document.body);
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FConnectionCountIndicator.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA81UQU7kMBC8zyv6gjRIGDmZXRbMhZ-gTtxJWji2ZTtMJoi_r-KEhQ2IFeKy8iXuqkpZrm7fPdCpCdhTBBfHK74u4WkHIM8uoJDyLG8AKjeK2KF2RwVyXaGtcC8v8jq_3QE87wB-fiI5-BEeMeyFuC9uaCrDNK66593lYi6z2LvIiZ1VEMhg4keaaZqjN3hSwNawJdEYGuc6Gm6t4ER9VFCTTRTmcotebezKU_bzqDXb9gN0W5kyv8L6oQ1usHor0RmvnXFhCxWL1AVNQUHhR4jOsN6w2jcsEVDzELc_OtxkTuNsEpEneodXr_iRuO3SO8biUg8hzufU1OBg0lzK99itouL2NYci5_C55x_yj01oWEVnhpRDS86r3EjzJixGcv7uMbRsRca_lNJ_kcmvb2fSZMaRdeoUlKX0uZePHScS0WNNCqwLPZp8izQmkftcgaEmZzcJtppGBaVczvt23Eo_wrUf_5rRyzKHdteTZoS9D9RQiCKQHmrSondLetaJBSJb03mOds35sE42Wu5xIb88GVdS9hEIIwm2wg3pZa7_5bbsP7T50gPyGwhAT1jHBAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FConnectionCountIndicator_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA81UQU7kMBC8zyv6gjRIGDmZXRbMhZ_gTtxJWji2ZTtMJoi_r_KEhQ2IFeKy8iXuqkpZrm7fPdCpCdhTBBfHK74u4WkHIM8uoJDyLG8AKjeK2KF2RwVyXaGtcC8v8jq_3QE87wB_fiI5_BEeMeyFuC9uaCrDNK66593lYi6z2LvIiZ1VEMhg4keaaZqjN3hSwNawJdEYGuc6Gm6t4ER9VFCTTRTmcotebezKU_bzqDXb9gN0W5kyv8L6oQ1usHor0RmvnXFhCxWL1AVNQUHhR4jOsN6w2jcsEVDzELc_OtxkTuNsEpEneodXr_iRuO3SO8biUg8hzufU1OBg0lzK99itouL2NYci5_C55x_yj01oWEVnhpRDS86r3EjzJixGcv7uMbRsRca_lNJ_kcmvb2fSZMaRdeoUlKX0uZePHScS0WNNCqwLPZp8izQmkftcgaEmZzcJtppGBaVczvt23Eo_wrUf_5rRyzKHdteTZoS9D9RQiCKQHmrSondLetaJBSJb03mOds35sE42Wu5xIb88GVdS9hEIIwm2wg3pZa7_5bbsP7T50gPyGwhAT1jHBAAA_22_7D = __webpack_require__(97986);
;// ./src/components/sessions/ConnectionCountIndicator.css.ts


var ConnectionCountIndicator_css_badge = 'osx6i80';
var icon = 'osx6i81';
var ConnectionCountIndicator_css_pulse = 'osx6i83';
var tooltip = 'osx6i84';
;// ./src/components/sessions/ConnectionCountIndicator.tsx
/* __next_internal_client_entry_do_not_use__ ConnectionCountIndicator auto */ 
// +feature: connection-count-indicator


// design/ux.md's "Rapid count oscillation" edge case (UX-AC-11): debounce the
// value that drives mount/unmount so a flapping reconnect doesn't produce a
// burst of separate mount/announce cycles — mirrors InputDropBadge.tsx's
// episode-coalescing precedent (research/ux.md §3).
const COALESCE_MS = 500;
// design/ux.md's interaction-flow Step 5: the departure ("2 -> 1") must be
// announced, not just silently unmounted — most screen readers only reliably
// announce content *mutations within a still-present* live region, not the
// disappearance of the whole node in the same commit. Hold the departure
// text in the DOM for one announcement cycle before actually unmounting.
const DEPARTURE_HOLD_MS = 1000;
function formatCount(count) {
    return `${count} connection${count === 1 ? "" : "s"} active`;
}
/**
 * Small, non-alarming indicator that another connection (tab, ssq-mux
 * attach, etc.) is attached to the same session (Epic 4.2, Story 4.2.2).
 * Renders only when count > 1 — the overwhelmingly common single-tab case
 * gets zero added visual surface (design/ux.md Surface 1, UX-AC-1).
 *
 * Follows TerminalOutput.tsx's existing reconnecting-banner convention
 * exactly: a single conditionally-mounted `role="status"`/`aria-live="polite"`
 * element, never `role="alert"`. Mounting only on the 1→2 transition (not
 * already true at initial page paint) is what gives "changes-only"
 * announcement semantics for free — a live region's initial-page-load
 * content is not narrated by assistive tech, only content that mutates
 * after the region already exists (Story 4.2.2 AC1).
 *
 * The reverse transition (settled count drops back to <= 1) does NOT unmount
 * immediately: design/ux.md's interaction-flow Step 5 requires an equivalent
 * departure signal ("must not go silent without any signal"), and most
 * screen readers only reliably announce content mutations within a
 * still-present live region, not a node's removal in the same commit. So the
 * departure updates this region's text first, holds it for
 * `DEPARTURE_HOLD_MS`, then unmounts.
 */ function ConnectionCountIndicator({ count, sizeMismatch, pulse }) {
    const [expanded, setExpanded] = (0,react.useState)(false);
    const [settledCount, setSettledCount] = (0,react.useState)(count);
    const [departureLabel, setDepartureLabel] = (0,react.useState)(null);
    const timerRef = (0,react.useRef)(null);
    const departureTimerRef = (0,react.useRef)(null);
    const prevSettledRef = (0,react.useRef)(settledCount);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (timerRef.current) {
            clearTimeout(timerRef.current);
        }
        timerRef.current = setTimeout(()=>setSettledCount(count), COALESCE_MS);
        return ()=>{
            if (timerRef.current) {
                clearTimeout(timerRef.current);
            }
        };
    }, [
        count
    ]);
    // Detect the settled-count transition back down to <= 1 and hold a
    // departure announcement in the same live region before unmounting
    // (design/ux.md Step 5).
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const prev = prevSettledRef.current;
        prevSettledRef.current = settledCount;
        const isVisible = settledCount !== undefined && settledCount > 1;
        if (isVisible) {
            // Back to (or still at) a real multi-connection count — cancel any
            // in-flight departure hold so the normal indicator takes over.
            if (departureTimerRef.current) {
                clearTimeout(departureTimerRef.current);
                departureTimerRef.current = null;
            }
            setDepartureLabel(null);
            return;
        }
        const wasVisible = prev !== undefined && prev > 1;
        if (wasVisible) {
            setDepartureLabel(formatCount(1));
            departureTimerRef.current = setTimeout(()=>{
                setDepartureLabel(null);
                departureTimerRef.current = null;
            }, DEPARTURE_HOLD_MS);
        }
    }, [
        settledCount
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        return ()=>{
            if (departureTimerRef.current) {
                clearTimeout(departureTimerRef.current);
            }
        };
    }, []);
    if (departureLabel !== null) {
        // Minimal departure render: same live region, no icon/tooltip — just the
        // text mutation a screen reader can pick up before the node disappears.
        return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
            className: ConnectionCountIndicator_css_badge,
            role: "status",
            "aria-live": "polite",
            "aria-label": departureLabel,
            "data-testid": "connection-count-indicator",
            children: departureLabel
        });
    }
    if (settledCount === undefined || settledCount <= 1) {
        return null;
    }
    const label = formatCount(settledCount);
    const tooltipText = sizeMismatch ? `${label}. Another connection has this session open at a different size.` : label;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
        className: pulse ? `${ConnectionCountIndicator_css_badge} ${ConnectionCountIndicator_css_pulse}` : ConnectionCountIndicator_css_badge,
        role: "status",
        "aria-live": "polite",
        "aria-label": label,
        "data-testid": "connection-count-indicator",
        "data-pulse": pulse ? "true" : undefined,
        tabIndex: 0,
        onMouseEnter: ()=>setExpanded(true),
        onMouseLeave: ()=>setExpanded(false),
        onFocus: ()=>setExpanded(true),
        onBlur: ()=>setExpanded(false),
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                className: icon,
                "aria-hidden": "true",
                children: "\uD83D\uDC65"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                children: settledCount
            }),
            expanded && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                className: tooltip,
                role: "tooltip",
                "data-testid": "connection-count-tooltip",
                children: tooltipText
            })
        ]
    });
}

;// ./src/components/sessions/useDropEpisodeCoalescer.ts
/* __next_internal_client_entry_do_not_use__ useDropEpisodeCoalescer auto */ 
/**
 * Coalesces rapid-fire `report(count)` calls into a single `onFlush(total)`
 * call after `windowMs` of inactivity, so a burst of N dropped keystrokes
 * (Story 2.3 — InputDropBadge) produces exactly one coalesced-episode
 * announcement rather than one per dropped keystroke.
 *
 * Internally: a ref-held running total plus a `setTimeout` that (re)schedules
 * on each `report()` call within the window. Once the window elapses without
 * a new `report()`, `onFlush(total)` fires once with the summed count and the
 * total resets to 0 — a `report()` call after that point starts a brand-new,
 * independent episode (design/ux.md §2.3 Case C: "replace, don't merge
 * across episodes").
 *
 * Extracted (Task 2.3.3a) into its own testable unit rather than left inline
 * in `TerminalOutput.tsx` (1500+ lines) — mirrors this plan's existing
 * precedent of pulling coalescing/latch logic out of large files
 * (`answerDialogOnce`, `runInputReadLoop`).
 */ function useDropEpisodeCoalescer(onFlush, windowMs) {
    const totalRef = (0,react.useRef)(0);
    const timerRef = (0,react.useRef)(null);
    // Keep the latest onFlush without forcing report() to be recreated on
    // every render (mirrors pushMessageRef's "read the ref, not a stale
    // closure" idiom in useTerminalStream.ts).
    const onFlushRef = (0,react.useRef)(onFlush);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        onFlushRef.current = onFlush;
    }, [
        onFlush
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        return ()=>{
            if (timerRef.current) {
                clearTimeout(timerRef.current);
                timerRef.current = null;
            }
        };
    }, []);
    const report = (0,react.useCallback)((count)=>{
        totalRef.current += count;
        if (timerRef.current) {
            clearTimeout(timerRef.current);
        }
        timerRef.current = setTimeout(()=>{
            const total = totalRef.current;
            totalRef.current = 0;
            timerRef.current = null;
            onFlushRef.current(total);
        }, windowMs);
    }, [
        windowMs
    ]);
    return report;
}

;// ./src/lib/terminal/EscapeSequenceParser.ts
/**
 * EscapeSequenceParser - Ensures ANSI escape sequences are not split across writes
 *
 * Terminal control codes (ANSI escape sequences) must be written atomically to xterm.js.
 * Splitting them mid-sequence causes terminal corruption. This parser detects partial
 * escape sequences at the end of data chunks and buffers them for the next write.
 *
 * Supported sequences:
 * - CSI: \x1b[ ... (letter)  - Cursor positioning, colors, attributes
 * - OSC: \x1b] ... \x07|\x1b\\ - Operating system commands
 * - Simple: \x1b(letter) - Single-char escapes
 *
 * References:
 * - xterm.js Flow Control Guide: https://xtermjs.org/docs/guides/flowcontrol/
 * - xterm.js Terminal API: https://xtermjs.org/docs/api/terminal/classes/terminal/
 * - ANSI Escape Sequences: https://en.wikipedia.org/wiki/ANSI_escape_code
 * - VT100 Control Sequences: https://vt100.net/docs/vt100-ug/chapter3.html
 */ class EscapeSequenceParser {
    static{
        // Allocated once at class definition; avoids a per-call object allocation in the hot path.
        this.STRING_MODE_CHARS = new Set([
            'P',
            '^',
            '_',
            'X'
        ]);
    }
    /**
   * Process data chunk and ensure escape sequences are not split.
   * Returns the complete data that can be safely written, buffering any
   * partial escape sequence for the next call.
   *
   * @param data - New data chunk to process
   * @returns Complete data safe to write (includes buffered partial from previous call)
   */ processChunk(data) {
        // Prepend any buffered partial sequence from previous call
        const fullData = this.partialSequence + data;
        this.partialSequence = "";
        const filtered = fullData; // No sequence stripping - xterm.js v6 handles ED2+ED3 correctly
        // Check if data ends with a partial escape sequence
        const partial = this.findPartialEscapeAtEnd(filtered);
        if (partial.length > 0) {
            // Buffer the partial sequence for next call
            this.partialSequence = partial;
            // Return data up to (but not including) the partial sequence
            return filtered.substring(0, filtered.length - partial.length);
        }
        // No partial sequence - safe to write all data
        return filtered;
    }
    /**
   * Get any buffered partial escape sequence (for debugging/testing)
   */ getBuffered() {
        return this.partialSequence;
    }
    /**
   * Clear any buffered data (call on disconnect/error)
   */ reset() {
        this.partialSequence = "";
    }
    /**
   * Detect partial escape sequence at end of string.
   * Returns the partial sequence string if found, empty string otherwise.
   *
   * Detection strategy:
   * 1. Scan backwards from end for ESC character (\x1b)
   * 2. Validate if this starts a complete or partial sequence
   * 3. CSI/OSC sequences need terminator, simple escapes need one more char
   */ findPartialEscapeAtEnd(data) {
        if (data.length === 0) return "";
        // Maximum length to scan backward - OSC title strings and DCS payloads can exceed 20 bytes
        const scanLength = Math.min(256, data.length);
        const startIndex = data.length - scanLength;
        // Scan backward for last ESC character
        let lastEscIndex = -1;
        for(let i = data.length - 1; i >= startIndex; i--){
            if (data.charCodeAt(i) === 0x1b) {
                lastEscIndex = i;
                break;
            }
        }
        if (lastEscIndex === -1) {
            return ""; // No escape sequence at end
        }
        // Extract potential escape sequence from last ESC to end
        const potentialSeq = data.substring(lastEscIndex);
        // Check if this is a complete sequence or partial
        if (this.isCompleteEscapeSequence(potentialSeq)) {
            return ""; // Complete sequence - no buffering needed
        }
        // Partial sequence - buffer it
        return potentialSeq;
    }
    /**
   * Check if escape sequence is complete (has terminator).
   *
   * ANSI escape sequence patterns:
   * - CSI: ESC [ ... (letter A-Z, a-z)
   * - OSC: ESC ] ... BEL(\x07) or ESC\
   * - Simple: ESC (letter)
   * - C1: ESC (special char in 0x40-0x5F range)
   */ isCompleteEscapeSequence(seq) {
        if (seq.length === 0 || seq.charCodeAt(0) !== 0x1b) {
            return true; // Not an escape sequence
        }
        if (seq.length === 1) {
            return false; // Just ESC - definitely partial
        }
        const secondChar = seq[1];
        // CSI sequence: ESC [
        if (secondChar === '[') {
            // CSI ends with a letter (A-Z, a-z) after optional parameters
            // Examples: ESC[31m (red), ESC[2K (clear line), ESC[10;20H (cursor position)
            return this.hasCSITerminator(seq);
        }
        // OSC sequence: ESC ]
        if (secondChar === ']') {
            // OSC ends with BEL (\x07) or ESC\ (\x1b\x5c)
            return this.hasOSCTerminator(seq);
        }
        // DCS (P), PM (^), APC (_), SOS (X) — all terminated by ST only (ESC \)
        if (EscapeSequenceParser.STRING_MODE_CHARS.has(secondChar)) {
            return seq.includes('\x1b\\');
        }
        // Simple escape: ESC + single char
        // If we have 2+ characters and second is not [ or ], it's likely complete
        // Examples: ESC 7 (save cursor), ESC 8 (restore cursor), ESC M (reverse index)
        if (seq.length === 2) {
            return true; // Simple 2-char escape is complete
        }
        // C1 control codes: ESC + char in 0x40-0x5F range
        const secondCharCode = seq.charCodeAt(1);
        if (secondCharCode >= 0x40 && secondCharCode <= 0x5F) {
            return true; // C1 sequence is complete
        }
        // Unknown sequence type - assume complete to avoid infinite buffering
        return true;
    }
    /**
   * Check if CSI sequence has terminator (letter A-Z, a-z).
   * CSI format: ESC [ (params) (letter)
   */ hasCSITerminator(seq) {
        if (seq.length < 3) return false; // Need at least ESC[X
        // Scan for terminator (letter)
        for(let i = 2; i < seq.length; i++){
            const code = seq.charCodeAt(i);
            // Terminators are letters A-Z (0x41-0x5A) or a-z (0x61-0x7A)
            if (code >= 0x41 && code <= 0x5A || code >= 0x61 && code <= 0x7A) {
                return true; // Found terminator
            }
            // Valid intermediate characters: digits, semicolon, ? , etc.
            // If we hit an invalid character, sequence is malformed
            const isValidIntermediate = code >= 0x30 && code <= 0x3F || // 0-9, :, ;, <, =, >, ?
            code >= 0x20 && code <= 0x2F; // space, !, ", #, $, %, &, ', (, ), *, +, comma, -, ., /
            if (!isValidIntermediate) {
                // Unexpected character - treat as complete to avoid blocking
                return true;
            }
        }
        // No terminator found yet - partial sequence
        return false;
    }
    /**
   * Check if OSC sequence has terminator (BEL or ESC\).
   * OSC format: ESC ] (params) BEL or ESC ]  (params) ESC \
   */ hasOSCTerminator(seq) {
        if (seq.length < 3) return false; // Need at least ESC]X
        // Check for BEL terminator (\x07)
        if (seq.indexOf('\x07') !== -1) {
            return true;
        }
        // Check for ESC\ terminator (\x1b\x5c or \x1b\\)
        if (seq.indexOf('\x1b\\') !== -1 || seq.indexOf('\x1b\x5c') !== -1) {
            return true;
        }
        // No terminator found - partial sequence
        return false;
    }
    constructor(){
        this.partialSequence = "";
    }
} /**
 * Example usage:
 *
 * const parser = new EscapeSequenceParser();
 *
 * // Chunk 1: Ends with partial CSI sequence
 * const chunk1 = "Hello \x1b[31";
 * const toWrite1 = parser.processChunk(chunk1); // Returns "Hello " (buffers "\x1b[31")
 *
 * // Chunk 2: Completes the CSI sequence
 * const chunk2 = "mWorld";
 * const toWrite2 = parser.processChunk(chunk2); // Returns "\x1b[31mWorld" (prepends buffered)
 */ 

;// ./src/lib/terminal/RedrawThrottler.ts
/**
 * RedrawThrottler - Coalesces rapid full-screen redraws to max 30 FPS.
 *
 * Claude performs complete screen redraws at 12-25 FPS, causing visible flicker.
 * This throttler holds rapid redraws and flushes the latest one at a capped rate.
 *
 * Split out of TerminalStreamManager.ts (which still owns the one instance per
 * manager) — it has no dependency on TerminalStreamManager's internals, only
 * the onFlush callback passed to its constructor.
 */ class RedrawThrottler {
    constructor(onFlush){
        this.pendingRedraw = null;
        // Cursor-up count (the \d+ in \x1b[\d+A) of the currently pending redraw —
        // identifies which screen region it repaints, so a same-region redraw can
        // be coalesced onto it while a different-region one cannot (see below).
        this.pendingRedrawUpCount = null;
        this.throttleTimer = null;
        this.throttleMs = 33 // ~30fps; coalesces burst full-screen redraws
        ;
        this.onFlush = onFlush;
    }
    process(chunk) {
        // Detect genuine full-screen redraws: cursor-up followed immediately by an erase sequence.
        // NOTE: \x1b[H (cursor-home) is intentionally excluded - it is also emitted during
        // incremental Ink-style renders and must not be classified as a full redraw.
        // Scoping the check to the first 32 bytes avoids false positives in large output chunks.
        const match = /^\x1b\[(\d+)A(?:\x1b\[2K|\x1b\[J)/.exec(chunk.substring(0, 32));
        if (!match) {
            this.flushPending();
            return chunk;
        }
        // \x1b[NA\x1b[2K (cursor-up N, erase ONE line) is also exactly how a multi-line
        // Ink-style TUI redraws a SINGLE targeted line (e.g. a spinner or token counter),
        // not just how it repaints the whole viewport — the leading bytes alone can't tell
        // the two apart. Coalescing is only safe when the new candidate targets the SAME
        // cursor-up region as the one already pending (repeated repaints of one region,
        // which is the flicker this throttler exists to suppress). A different region
        // arriving mid-window is flushed immediately instead of silently overwriting the
        // pending one — otherwise the pending region's update is dropped forever, since
        // nothing else resends it.
        const upCount = Number(match[1]);
        if (this.pendingRedraw !== null && upCount !== this.pendingRedrawUpCount) {
            this.flushPending();
        }
        this.pendingRedraw = chunk;
        this.pendingRedrawUpCount = upCount;
        if (!this.throttleTimer) {
            this.throttleTimer = setTimeout(()=>{
                this.flushPending();
            }, this.throttleMs);
        }
        return null; // Don't output yet
    }
    flushPending() {
        if (this.pendingRedraw) {
            this.onFlush(this.pendingRedraw);
            this.pendingRedraw = null;
            this.pendingRedrawUpCount = null;
        }
        if (this.throttleTimer) {
            clearTimeout(this.throttleTimer);
            this.throttleTimer = null;
        }
    }
    cleanup() {
        this.flushPending();
    }
}

;// ./src/lib/terminal/TerminalDebugMonitor.ts
/** True when verbose terminal debug logging is enabled via localStorage. */ function isDebugTerminalEnabled() {
    return  false && 0;
}
/**
 * TerminalDebugMonitor - optional write/refresh monkey-patching that logs
 * timing and call-site info when the `debug-terminal` localStorage flag is
 * set. Debug-only instrumentation, not on the hot write path.
 *
 * Split out of TerminalStreamManager.ts, which owns one instance per manager
 * and drives it purely through install()/restore().
 */ class TerminalDebugMonitor {
    /** Wrap terminal.write/refresh to log timing/call-site info. Idempotent. */ install(terminal) {
        if (this.installed) return;
        this.originalWrite = terminal.write.bind(terminal);
        this.originalRefresh = terminal.refresh.bind(terminal);
        terminal.write = this.wrapWrite(terminal);
        terminal.refresh = this.wrapRefresh(terminal);
        this.installed = true;
        if (isDebugTerminalEnabled()) {
            console.log('[TerminalStreamManager] Refresh and write monitoring installed');
        }
    }
    /** Restore the original terminal.write/refresh captured by install(). Idempotent. */ restore(terminal) {
        if (!this.installed || !this.originalWrite || !this.originalRefresh) return;
        terminal.write = this.originalWrite;
        terminal.refresh = this.originalRefresh;
        this.installed = false;
        // Null out refs to terminal internals to prevent GC retention (Pitfall #7):
        // when the terminal is disposed while the manager is still alive, these refs
        // hold closed terminal objects preventing garbage collection.
        this.originalWrite = null;
        this.originalRefresh = null;
    }
    wrapWrite(terminal) {
        return (data, callback)=>{
            const now = performance.now();
            const timeSinceLastWrite = now - this.lastWriteTime;
            this.lastWriteTime = now;
            this.writeCount++;
            if (isDebugTerminalEnabled()) {
                console.log('[XtermWrite]', {
                    writeCount: this.writeCount,
                    dataLength: typeof data === 'string' ? data.length : data.byteLength,
                    timeSinceLastWrite: `${timeSinceLastWrite.toFixed(2)}ms`,
                    cursorY: terminal.buffer.active.cursorY,
                    timestamp: new Date().toISOString()
                });
            }
            return this.originalWrite?.(data, callback);
        };
    }
    wrapRefresh(terminal) {
        return (start, end)=>{
            if (isDebugTerminalEnabled()) {
                const stackTrace = new Error().stack;
                const caller = stackTrace?.split('\n')[2]?.trim() || 'unknown';
                const timeSinceLastWrite = performance.now() - this.lastWriteTime;
                console.log('[XtermRefresh] Refresh called', {
                    start,
                    end,
                    rows: terminal.rows,
                    timeSinceLastWrite: `${timeSinceLastWrite.toFixed(2)}ms`,
                    recentWrites: this.writeCount,
                    caller: caller.replace(/^at /, ''),
                    possibleRaceCondition: timeSinceLastWrite < 50,
                    timestamp: new Date().toISOString()
                });
                this.writeCount = 0;
            }
            return this.originalRefresh?.(start, end);
        };
    }
    constructor(){
        this.originalWrite = null;
        this.originalRefresh = null;
        this.lastWriteTime = 0;
        this.writeCount = 0;
        this.installed = false;
    }
}

;// ./src/lib/terminal/TerminalStreamManager.ts
/**
 * TerminalStreamManager - Write buffering, watermark flow control, chunked writes,
 * redraw throttling, and escape sequence safety for xterm.js.
 *
 * This is a pure imperative class (no React dependency). It encapsulates the
 * terminal write pipeline that was previously scattered across TerminalOutput.tsx.
 *
 * Lifecycle: Create an instance in a ref, call methods from effects, call cleanup() on unmount.
 */ 


// ---- Constants ----
const HIGH_WATERMARK = 100000; // 100KB - pause when buffer exceeds this
const LOW_WATERMARK = 10000; // 10KB - resume when buffer drops below this
const CHUNK_SIZE = 16384; // 16KB chunks
const CHUNK_DELAY_MS = 0; // Yield to event loop between chunks
/**
 * Mirrors connectrpc_websocket.go's `ansiSnapshotPrefix` (DECSTR + erase-screen +
 * cursor-home). The server prepends this exact byte sequence to every full-pane
 * replacement snapshot it sends — the post-resize capture, visibility/focus resync,
 * and reconnect snapshot all use it — so its presence at the start of a chunk is a
 * reliable, trigger-independent signal that this chunk replaces the whole screen
 * rather than appending to it.
 *
 * Checked once, here, inside write() — the single funnel every output path (live
 * streaming, resize snapshots, resync snapshots, and the RESIZING-queue flush in
 * TerminalOutput.tsx) already goes through — rather than at each call site, so a
 * future call site can't reintroduce the stale-buffer-overlap bug by forgetting to
 * check it.
 */ const ANSI_SNAPSHOT_PREFIX = "\x1b[!p\x1b[2J\x1b[H";
class TerminalStreamManager {
    constructor(terminal, sendFlowControl){
        // Pending write operations tracking
        this.pendingWrites = 0;
        this.totalBytesWritten = 0;
        this.totalBytesCompleted = 0;
        // Watermark flow control
        this.watermark = 0;
        this.isPaused = false;
        // Chunked write queue
        this.writeQueue = [];
        this.isProcessingQueue = false;
        // Escape sequence parser
        this.escapeParser = new EscapeSequenceParser();
        // Debug instrumentation (write/refresh monkey-patching), gated on the
        // `debug-terminal` localStorage flag — see installDebugMonitor().
        this.debugMonitor = new TerminalDebugMonitor();
        // Write-lock: prevents live output from being written while initial/paged history is loading.
        // Live writes received during the lock are queued and flushed once the history write completes.
        this.isWritingInitialContent = false;
        this.pendingLiveWrites = [];
        // First output tracking
        this.firstOutputReceived = false;
        this.onFirstOutput = null;
        // Invoked whenever write() detects a full-pane replacement snapshot (see
        // ANSI_SNAPSHOT_PREFIX), after this.terminal.clear() has already run — lets the
        // caller reset state it owns that the manager doesn't (e.g. TerminalOutput.tsx's
        // scrollback-paging refs).
        this.onFullSnapshot = null;
        // Story 1.4.0 — client-local mirror of the server's AltScreenTracker (Task 1.1.1a),
        // observed independently off the same escape sequences already flowing through
        // write() rather than round-tripped over the wire. Purely a local scroll-up-trigger
        // decision (see TerminalOutput.tsx's wheel listener / useTerminalGestures wiring);
        // the server keeps its own copy for the safety-gate role, and the two are not meant
        // to be reconciled — they observe the same bytes independently by design.
        this.altScreenActive = false;
        this.onAltScreenChange = null;
        // Story 1.4.1 — registered handler for a captured AppScrollbackResponse frame.
        // Deliberately dispatched through handleAppScrollback(), a distinct entry point
        // from write()'s ANSI_SNAPSHOT_PREFIX sniff, so a concurrent resize resync's
        // onFullSnapshot can never fire for this message type and vice versa
        // (research/architecture.md §6).
        this.onAppScrollback = null;
        // Story 3 (Task 3.6 / Bug 3) — set by cleanup(). See cleanup()'s doc comment.
        this.disposed = false;
        this.terminal = terminal;
        this.sendFlowControl = sendFlowControl;
        this.redrawThrottler = new RedrawThrottler((data)=>{
            const safeOutput = this.escapeParser.processChunk(data);
            this.handleProcessedOutput(safeOutput);
        });
    }
    /** Set a callback invoked once on first output received. */ setOnFirstOutput(cb) {
        this.onFirstOutput = cb;
    }
    /** Set a callback invoked whenever a full-pane replacement snapshot is detected in write(). */ setOnFullSnapshot(cb) {
        this.onFullSnapshot = cb;
    }
    /**
   * Set a callback invoked whenever altScreenActive changes value (Task 1.4.0a) —
   * called only on an actual transition, mirroring the server-side
   * `AltScreenTracker.Observe`'s `changed` return convention.
   */ setOnAltScreenChange(cb) {
        this.onAltScreenChange = cb;
    }
    /**
   * Marks altScreenActive true from the server-authoritative
   * TerminalOutput.alt_screen_active hint (only ever present on an
   * initial-connect/post-resize snapshot — see useTerminalStream.ts's
   * onAltScreenActiveHint doc comment) rather than inferring it from
   * scanning content bytes, which a capture-pane-derived snapshot can never
   * carry the DECSET 1049h marker for. Fires onAltScreenChange only on an
   * actual transition, mirroring the server-side `AltScreenTracker.Observe`'s
   * `changed` return convention.
   */ setAltScreenActive() {
        if (!this.altScreenActive) {
            this.altScreenActive = true;
            this.onAltScreenChange?.(true);
        }
    }
    /** See setAltScreenActive()'s doc comment — the inactive counterpart. */ setAltScreenInactive() {
        if (this.altScreenActive) {
            this.altScreenActive = false;
            this.onAltScreenChange?.(false);
        }
    }
    /** Set a callback invoked when handleAppScrollback() dispatches a captured frame (Task 1.4.1a). */ setOnAppScrollback(cb) {
        this.onAppScrollback = cb;
    }
    /**
   * Dispatch a captured AppScrollbackResponse frame to the registered
   * onAppScrollback callback. Never routes through write()/onFullSnapshot —
   * see this.onAppScrollback's doc comment.
   */ handleAppScrollback(frame) {
        this.onAppScrollback?.(frame);
    }
    /** Inject a SerializeAddon for prependScrollbackBatch (serialize-clear-rewrite pattern). */ setSerializeAddon(addon) {
        this.serializeAddon = addon;
    }
    /** Update the sendFlowControl callback (e.g., after reconnection). */ updateSendFlowControl(fn) {
        this.sendFlowControl = fn;
    }
    /** Install debug instrumentation (write/refresh monkey-patching). See TerminalDebugMonitor. */ installDebugMonitor() {
        this.debugMonitor.install(this.terminal);
    }
    /** True when verbose terminal debug logging is enabled via localStorage. */ isDebugTerminalEnabled() {
        return  false && 0;
    }
    /**
   * Write output to the terminal with throttling, escape sequence safety, and flow control.
   * This is the primary entry point for streaming output.
   */ write(output) {
        if (this.disposed) return;
        // Write-lock: if writeInitialContent or prependScrollbackBatch is in progress,
        // queue live output to avoid interleaving history with live data (Pitfall #1 and #8).
        if (this.isWritingInitialContent) {
            this.pendingLiveWrites.push(output);
            return;
        }
        // A full-pane replacement snapshot must replace the screen, not append to it —
        // see ANSI_SNAPSHOT_PREFIX's doc comment. Without this, stale rows from before
        // the snapshot (reflowed under the terminal's previous wrap state, or content
        // the user had scrolled past) sit underneath the fresh content instead of being
        // replaced by it.
        if (output.startsWith(ANSI_SNAPSHOT_PREFIX)) {
            this.terminal.clear();
            this.onFullSnapshot?.();
        }
        // Track first output
        if (!this.firstOutputReceived) {
            this.firstOutputReceived = true;
            this.onFirstOutput?.();
        }
        // Throttle rapid full-screen redraws to prevent flickering
        const result = this.redrawThrottler.process(output);
        if (result) {
            const safeOutput = this.escapeParser.processChunk(result);
            this.handleProcessedOutput(safeOutput);
        }
    }
    /**
   * Write initial content (e.g., from currentPaneResponse).
   * Clears terminal, enqueues content via chunked write, scrolls to bottom.
   * Holds a write-lock for the duration to prevent live output from interleaving.
   *
   * @returns Promise that resolves when the write completes.
   */ async writeInitialContent(content) {
        if (this.disposed) return;
        this.isWritingInitialContent = true;
        try {
            this.terminal.clear();
            // Note: initial/history content here is a tmux capture-pane-derived
            // snapshot, which can never itself carry a DECSET 1049h marker (it's a
            // rendered text snapshot, not a replay of the raw byte stream) — so
            // scanning it for alt-screen transitions would never find anything.
            // setAltScreenActive()/setAltScreenInactive(), driven by the
            // server-authoritative TerminalOutput.alt_screen_active field, is what
            // actually establishes correct alt-screen state on connect; see their
            // doc comments.
            await this.enqueueWrite(content);
            this.terminal.scrollToBottom();
            // Delayed scrolls in case content is still rendering.
            // Large scrollback can take >100ms to fully render on slower machines.
            setTimeout(()=>this.terminal.scrollToBottom(), 10);
            setTimeout(()=>this.terminal.scrollToBottom(), 100);
            setTimeout(()=>this.terminal.scrollToBottom(), 500);
        } finally{
            this.isWritingInitialContent = false;
            // Flush any live writes that arrived during history loading (in order)
            const pending = this.pendingLiveWrites.splice(0);
            for (const chunk of pending){
                this.write(chunk);
            }
        }
    }
    /**
   * Prepend historical scrollback above the current terminal content.
   * Uses the serialize-clear-rewrite pattern (ADR-010):
   *   1. Serialize current buffer (if SerializeAddon available)
   *   2. Clear terminal
   *   3. Write older history
   *   4. Write saved current content
   *   5. Scroll to bottom
   *
   * Holds a write-lock for the duration to prevent live output from interleaving.
   *
   * @returns Promise that resolves when the write completes.
   */ async prependScrollbackBatch(content) {
        if (this.disposed) return;
        this.isWritingInitialContent = true;
        try {
            if (!this.serializeAddon) {
                // Degraded mode: SerializeAddon not available — cannot serialize current buffer before
                // clearing, so we skip the clear and simply append the history content. The history
                // will appear after the current content (wrong order) but current content is preserved.
                // This is preferable to clearing and losing the existing terminal output (Bug 6 fix).
                console.warn('[TerminalStreamManager] SerializeAddon not available — appending history without clear (degraded mode)');
                await this.enqueueWrite(content);
                return;
            }
            // Serialize current buffer state before clearing
            const serialized = this.serializeAddon.serialize();
            this.terminal.clear();
            await this.enqueueWrite(content);
            if (serialized) {
                await this.enqueueWrite(serialized);
            }
            this.terminal.scrollToBottom();
        } finally{
            this.isWritingInitialContent = false;
            // Flush any live writes that arrived during history loading (in order)
            const pending = this.pendingLiveWrites.splice(0);
            for (const chunk of pending){
                this.write(chunk);
            }
        }
    }
    /**
   * Task 1.4.0a — track altScreenActive off the same enter/exit markers the
   * needsRefresh scan in handleProcessedOutput already watches for exit.
   * lastIndexOf + compare (rather than two independent .includes() checks)
   * resolves the rare case where a single throttled chunk contains both an
   * exit and a re-entry by picking whichever marker occurs later in the chunk.
   */ updateAltScreenActive(safeOutput) {
        const altEnterIdx = Math.max(safeOutput.lastIndexOf('\x1b[?1049h'), safeOutput.lastIndexOf('\x1b[?47h'));
        const altExitIdx = Math.max(safeOutput.lastIndexOf('\x1b[?1049l'), safeOutput.lastIndexOf('\x1b[?47l'));
        if (altEnterIdx < 0 && altExitIdx < 0) return;
        if (altEnterIdx > altExitIdx) {
            this.setAltScreenActive();
        } else {
            this.setAltScreenInactive();
        }
    }
    /**
   * Handle processed output by routing through the appropriate write path
   * (raw direct/chunked or state batching).
   */ handleProcessedOutput(safeOutput) {
        if (safeOutput.length === 0) return;
        this.updateAltScreenActive(safeOutput);
        // Detect terminal mode transitions that may need a refresh
        const needsRefresh = safeOutput.includes('\x1b[?1049l') || safeOutput.includes('\x1b[?47l') || safeOutput.includes('\x1b[?2026l') || safeOutput.includes('\x1b[?25h');
        if (needsRefresh) {
            requestAnimationFrame(()=>{
                this.terminal.refresh(0, this.terminal.rows - 1);
                if (false) {}
            });
        }
        // For small writes, write directly for lowest latency with flow control tracking
        if (safeOutput.length <= CHUNK_SIZE) {
            this.writeDirectWithFlowControl(safeOutput);
        } else {
            // Large write - use chunked queue to prevent UI freezing
            this.enqueueWrite(safeOutput);
        }
    }
    /** Direct write with watermark tracking (fast path for small data). */ writeDirectWithFlowControl(data) {
        this.pendingWrites++;
        this.totalBytesWritten += data.length;
        this.watermark += data.length;
        // Check if we should pause upstream
        if (this.watermark > HIGH_WATERMARK && !this.isPaused) {
            this.isPaused = true;
            console.warn(`[FlowControl] HIGH WATERMARK EXCEEDED - Pausing stream (watermark: ${this.watermark} bytes)`);
            this.sendFlowControl(true, this.watermark);
        }
        this.terminal.write(data, ()=>{
            this.pendingWrites = Math.max(0, this.pendingWrites - 1);
            this.totalBytesCompleted += data.length;
            this.watermark = Math.max(0, this.watermark - data.length);
            if (this.watermark < LOW_WATERMARK && this.isPaused) {
                this.isPaused = false;
                console.log(`[FlowControl] LOW WATERMARK REACHED - Resuming stream (watermark: ${this.watermark} bytes)`);
                this.sendFlowControl(false, this.watermark);
            }
            if (false) {}
        });
    }
    /** Enqueue data for chunked writing (large data path). */ enqueueWrite(data) {
        return new Promise((resolve)=>{
            this.writeQueue.push({
                data,
                resolve
            });
            this.processWriteQueue();
        });
    }
    /** Process the write queue sequentially with chunking. */ async processWriteQueue() {
        if (this.isProcessingQueue || this.writeQueue.length === 0) {
            return;
        }
        this.isProcessingQueue = true;
        while(this.writeQueue.length > 0){
            if (this.disposed) {
                // Drop remaining queued items without writing — cleanup() already
                // cleared writeQueue, but a chunked item mid-flight re-checks this
                // flag between chunks (writeChunkedWithFlowControl), so this outer
                // guard only matters if cleanup() ran between two top-level queue
                // items.
                this.writeQueue.length = 0;
                break;
            }
            const item = this.writeQueue[0];
            const data = item.data;
            if (data.length <= CHUNK_SIZE) {
                await this.writeWithFlowControl(data);
            } else {
                await this.writeChunkedWithFlowControl(data);
            }
            this.writeQueue.shift();
            item.resolve();
        }
        this.isProcessingQueue = false;
    }
    /**
   * Write large data in CHUNK_SIZE pieces via writeWithFlowControl(), yielding
   * to the UI event loop between chunks. Re-checks `disposed` before every
   * chunk (Task 3.6 / Bug 3) — dispose() can land between any two chunks of a
   * large writeInitialContent()/prependScrollbackBatch() write, and the whole
   * point is to stop mid-item, not just between queue items.
   */ async writeChunkedWithFlowControl(data) {
        const totalChunks = Math.ceil(data.length / CHUNK_SIZE);
        if (this.isDebugTerminalEnabled()) {
            console.log(`[FlowControl] Chunking large write: ${data.length} bytes into ${totalChunks} chunks`);
        }
        for(let i = 0; i < data.length; i += CHUNK_SIZE){
            if (this.disposed) break;
            const chunk = data.slice(i, Math.min(i + CHUNK_SIZE, data.length));
            const chunkIndex = Math.floor(i / CHUNK_SIZE) + 1;
            await this.writeWithFlowControl(chunk, `chunk ${chunkIndex}/${totalChunks}`);
            // Yield to UI between chunks
            if (i + CHUNK_SIZE < data.length) {
                await this.yieldToUI();
            }
        }
        if (this.isDebugTerminalEnabled()) {
            console.log(`[FlowControl] Completed chunked write: ${data.length} bytes`);
        }
    }
    /** Yield to the UI event loop between chunks (next rAF, or a fixed delay if configured). */ yieldToUI() {
        return new Promise((resolve)=>{
            if (CHUNK_DELAY_MS > 0) {
                setTimeout(resolve, CHUNK_DELAY_MS);
            } else {
                requestAnimationFrame(()=>resolve());
            }
        });
    }
    /**
   * Write one piece of data to the terminal with watermark-based flow-control
   * pause/resume around it. Shared by processWriteQueue()'s small-direct-write
   * path and writeChunkedWithFlowControl()'s per-chunk path — both need the
   * identical pause-before/resume-after shape, previously duplicated inline.
   *
   * @param logContext optional context (e.g. "chunk 2/5") interpolated into
   *   the watermark pause/resume debug log lines.
   */ writeWithFlowControl(piece, logContext) {
        return new Promise((resolve)=>{
            this.watermark += piece.length;
            if (this.watermark > HIGH_WATERMARK && !this.isPaused) {
                this.isPaused = true;
                console.warn(`[FlowControl] HIGH WATERMARK EXCEEDED${logContext ? ` during ${logContext}` : ''} - Pausing stream (watermark: ${this.watermark} bytes)`);
                this.sendFlowControl(true, this.watermark);
            }
            this.terminal.write(piece, ()=>{
                this.watermark = Math.max(0, this.watermark - piece.length);
                if (this.watermark < LOW_WATERMARK && this.isPaused) {
                    this.isPaused = false;
                    console.log(`[FlowControl] LOW WATERMARK REACHED${logContext ? ` after ${logContext}` : ''} - Resuming stream (watermark: ${this.watermark} bytes)`);
                    this.sendFlowControl(false, this.watermark);
                }
                resolve();
            });
        });
    }
    /** Clean up all pending state on unmount. */ cleanup() {
        // Reset escape sequence parser
        this.escapeParser.reset();
        // Cleanup redraw throttler
        this.redrawThrottler.cleanup();
        // Restore original terminal methods if debug monitoring was installed
        // (also nulls out the captured refs — see TerminalDebugMonitor.restore's
        // Pitfall #7 doc comment on GC retention of a disposed terminal).
        this.debugMonitor.restore(this.terminal);
        // Clear any pending live writes
        this.pendingLiveWrites.length = 0;
        // Task 3.6 / Bug 3 — stop issuing further terminal.write() calls after
        // cleanup(). Pooled terminals (TerminalPool.tsx) reuse the underlying
        // xterm.js Terminal across sessions, so a manager whose owning
        // TerminalOutput has already unmounted mid-chunked-write must not keep
        // writing into a Terminal that may now belong to a different session.
        // Checked in write()/writeInitialContent()/prependScrollbackBatch() and
        // between chunks in writeChunkedWithFlowControl() — never mid-chunk (an
        // in-flight terminal.write() call always completes; only the *next* one
        // is skipped).
        this.disposed = true;
        this.writeQueue.length = 0;
    }
    // ---- Test/debug accessors ----
    /** Get current watermark level (for testing). */ getWatermark() {
        return this.watermark;
    }
    /** Check if flow control is paused (for testing). */ getIsPaused() {
        return this.isPaused;
    }
}
// Export constants for testing


;// ./src/lib/terminal/ForwardEchoGuard.ts
/**
 * Recognizes the redraw echo of a scroll-forward (DELIVERED/AT_TOP) in the live
 * output stream, so TerminalOutput doesn't mistake it for a genuine live resume
 * (Ctrl+L) and drop the "viewing history" banner.
 *
 * ForwardScroll's PageUp send (session/instance_scroll_forward.go) causes a real
 * redraw that echoes back as a normal frame. Content comparison, not a time
 * window or frame count, is what's invariant: the echo's arrival timing isn't
 * guaranteed (some redraws never echo at all), but it always redraws the same
 * pane content that was just captured.
 */ // Generous headroom over any real single-pane capture.
const MAX_ECHO_BUFFER = 20000;
/**
 * Strips ANSI CSI/OSC sequences (full ECMA-48 grammar, not just digits/semicolons
 * -- narrower patterns miss e.g. DECSTR's `\x1b[!p`) and all whitespace, so two
 * redraws of the same pane compare equal despite differing line-wrap columns.
 * Deliberately not truncated: with no alt-screen scrollback, the server's capture
 * can start at a different offset into the same content than the live echo.
 */ function contentSignature(raw) {
    return raw.replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "").replace(/\x1b\][^\x07]*(\x07|\x1b\\)/g, "").replace(/\s+/g, "");
}
/**
 * Longest N such that signature.slice(0, N) is a contiguous substring of
 * haystack. Monotonic in N, so a binary search needs O(log N) includes() calls
 * rather than an O(N^2) character scan.
 */ function longestSignaturePrefixContained(signature, haystack) {
    let lo = 0;
    let hi = signature.length;
    while(lo < hi){
        const mid = Math.ceil((lo + hi) / 2);
        if (haystack.includes(signature.slice(0, mid))) {
            lo = mid;
        } else {
            hi = mid - 1;
        }
    }
    return lo;
}
class ForwardEchoGuard {
    /** Records the content of a just-delivered forward; empty content disarms the guard. */ record(content) {
        this.signature = content ? contentSignature(content) : null;
        this.buffer = "";
        this.matchedPrefixLen = 0;
    }
    /** Disarms the guard (e.g. after a full-pane snapshot makes the forward stale). */ clear() {
        this.signature = null;
    }
    /** Whether `output` is recognizable as the echo of the last recorded forward. */ isSelfEcho(output) {
        const signature = this.signature;
        if (!signature) return false;
        const outSig = contentSignature(output);
        if (outSig.length === 0) return true; // no content to judge either way; assume still mid-echo
        const buffered = (this.buffer + outSig).slice(-MAX_ECHO_BUFFER);
        const matchedLen = longestSignaturePrefixContained(signature, buffered);
        if (matchedLen >= signature.length) {
            // Whole signature seen: resolved, so the next frame is judged fresh.
            this.buffer = "";
            this.matchedPrefixLen = 0;
            return true;
        }
        if (matchedLen > this.matchedPrefixLen) {
            // Genuine progress toward completing the same multi-frame echo.
            this.buffer = buffered;
            this.matchedPrefixLen = matchedLen;
            return true;
        }
        // No progress: judge this frame on its own content, not the stale buffer.
        const isEchoAlone = outSig.includes(signature) || signature.includes(outSig);
        this.buffer = isEchoAlone ? outSig : "";
        this.matchedPrefixLen = isEchoAlone ? longestSignaturePrefixContained(signature, outSig) : 0;
        return isEchoAlone;
    }
    constructor(){
        this.signature = null;
        // A redraw echo can legally arrive split across several frames, where no
        // single frame is a substring match but their concatenation is. Progress must
        // strictly increase each frame to keep accumulating; a frame that doesn't
        // extend the match is judged alone and the buffer resets, so a stale partial
        // match can't "stick" and misclassify a later genuine live resume.
        this.buffer = "";
        this.matchedPrefixLen = 0;
    }
}

;// ./src/components/sessions/useScrollLoadingPill.ts
/* __next_internal_client_entry_do_not_use__ useScrollLoadingPill auto */ 
const SHOW_DELAY_MS = 150;
const STALL_AFTER_MS = 8000;
/**
 * Story 1.4.5 — scroll-forward loading pill state.
 *
 * Shared by BOTH the tmux-native paged-scrollback trigger and Story 1.4.0's
 * app-forwarded alt-screen triggers (design/ux.md Surface 1), so the pill can't
 * drift out of sync with which trigger fired. Owns its timers and clears them on
 * unmount.
 */ function useScrollLoadingPill() {
    const [pillState, setPillState] = (0,react.useState)({
        visible: false,
        stalled: false
    });
    const showTimerRef = (0,react.useRef)(null);
    const stallTimerRef = (0,react.useRef)(null);
    const clearScrollLoadingTimers = (0,react.useCallback)(()=>{
        if (showTimerRef.current) {
            clearTimeout(showTimerRef.current);
            showTimerRef.current = null;
        }
        if (stallTimerRef.current) {
            clearTimeout(stallTimerRef.current);
            stallTimerRef.current = null;
        }
    }, []);
    // Called from every response handler (tmux-native ScrollbackResponse and every
    // AppScrollbackResponse outcome) so the pill never persists past outcome
    // delivery (Task 1.4.5b).
    const clearScrollLoadingState = (0,react.useCallback)(()=>{
        clearScrollLoadingTimers();
        setPillState({
            visible: false,
            stalled: false
        });
    }, [
        clearScrollLoadingTimers
    ]);
    // Starts the show-delay / stalled timers; the caller then sends the request.
    // `program`, if known, drives the pill's app-forwarded copy; omitted entirely
    // for the tmux-native path.
    const startScrollLoadingTimers = (0,react.useCallback)((program)=>{
        clearScrollLoadingTimers();
        showTimerRef.current = setTimeout(()=>{
            setPillState({
                visible: true,
                stalled: false,
                program
            });
        }, SHOW_DELAY_MS);
        stallTimerRef.current = setTimeout(()=>{
            setPillState({
                visible: true,
                stalled: true,
                program
            });
        }, STALL_AFTER_MS);
    }, [
        clearScrollLoadingTimers
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>clearScrollLoadingTimers, [
        clearScrollLoadingTimers
    ]);
    return {
        pillState,
        clearScrollLoadingState,
        startScrollLoadingTimers
    };
}

;// ./src/lib/terminal/TerminalDimensionCache.ts
/**
 * Constants for minimum and default terminal dimensions.
 *
 * Dimensions smaller than MIN_COLS/MIN_ROWS are transient (e.g., from xterm.js before
 * the CSS container has finished laying out). Caching or connecting at those dimensions
 * produces a garbled terminal on the next view. The first resize event often fires at
 * e.g., 10x6 before layout is complete.
 *
 * We also reject xterm.js default dimensions (80x24) to prevent cache corruption — the
 * logic in saveDimensions will not overwrite a larger existing cache with smaller
 * dimensions from the xterm.js default mount behavior.
 */ const MIN_COLS = 30;
const MIN_ROWS = 10;
/** xterm.js's default terminal dimensions — rejected to prevent cache corruption. */ const XTERM_DEFAULT_COLS = 80;
const XTERM_DEFAULT_ROWS = 24;
/**
 * Retrieve cached terminal dimensions for a given session.
 *
 * @param sessionId - The session identifier used as the cache key
 * @returns The cached dimensions, or null if not found or on error
 */ function getCachedDimensions(sessionId) {
    if (true) return null;
    try {
        const key = `terminal-dimensions-${sessionId}`;
        const cached = localStorage.getItem(key);
        if (cached) {
            const dims = JSON.parse(cached);
            console.log(`[TerminalDimensionCache] Loaded cached dimensions for ${sessionId}: ${dims.cols}x${dims.rows} (cell: ${dims.cellWidth?.toFixed(2)}x${dims.cellHeight?.toFixed(2)})`);
            return dims;
        }
    } catch (err) {
        console.warn('[TerminalDimensionCache] Failed to load cached dimensions:', err);
        return null;
    }
    return null;
}
/**
 * Get the current window dimensions with validation against minimum values.
 *
 * @returns Object with cols, rows, cellWidth, cellHeight, or null if dimensions invalid
 */ function getCurrentWindowDimensions() {
    if (true) return null;
    // Get window inner dimensions
    const cols = Math.max(MIN_COLS, Math.min(window.innerWidth / 8, XTERM_DEFAULT_COLS));
    const rows = Math.max(MIN_ROWS, Math.min(window.innerHeight / 18, XTERM_DEFAULT_ROWS));
    console.log(`[TerminalDimensionCache] Calculated window dimensions: ${cols}x${rows}`);
    return {
        cols,
        rows
    };
}
/**
 * Validate cached terminal dimensions against current font configuration.
 *
 * @param cached - Cached dimensions from localStorage (must include cols, rows, optionally cellWidth/cellHeight, fontSize/fontFamily)
 * @param fontSize - Current font size in px (from DEFAULT_TERMINAL_CONFIG)
 * @param fontFamily - Current font family (from DEFAULT_TERMINAL_CONFIG)
 * @returns Validated dimensions, or null if cache is stale/corrupt
 */ function validateCellDimensions(cached, fontSize, fontFamily) {
    // Create a copy to avoid mutating the cached value
    const validated = {
        cols: cached.cols,
        rows: cached.rows
    };
    // Font configuration changed — invalidate cell dimensions
    if (cached.fontSize != null && cached.fontFamily != null && (cached.fontSize !== fontSize || cached.fontFamily !== fontFamily)) {
        console.log(`[TerminalDimensionCache] Invalidating stale cache due to font config change: ` + `${cached.fontSize}px/${cached.fontFamily} → ${fontSize}px/${fontFamily}`);
        return null;
    }
    // Preserve cell dimensions when font unchanged, but reset font-related fields
    if (cached.cellWidth != null && cached.cellHeight != null) {
        validated.cellWidth = cached.cellWidth;
        validated.cellHeight = cached.cellHeight;
        validated.fontSize = fontSize;
        validated.fontFamily = fontFamily;
    }
    console.log(`[TerminalDimensionCache] Validated cached dimensions: ${validated.cols}x${validated.rows}`);
    return validated;
}
/** Rejects xterm.js defaults (80x24) and sub-minimum dimensions so they never corrupt the cache. */ function isValidToCache(cols, rows) {
    if (cols === XTERM_DEFAULT_COLS && rows === XTERM_DEFAULT_ROWS) {
        console.log(`[TerminalDimensionCache] Rejecting xterm.js default dimensions (${cols}x${rows}) for caching`);
        return false;
    }
    if (cols < MIN_COLS || rows < MIN_ROWS) {
        console.log(`[TerminalDimensionCache] Rejecting sub-minimum dimensions (${cols}x${rows}) for caching`);
        return false;
    }
    return true;
}
/**
 * Save terminal dimensions to localStorage for a given session.
 *
 * @param sessionId - The session identifier used as the cache key
 * @param cols - Number of terminal columns
 * @param rows - Number of terminal rows
 * @param options - Optional configuration for cell dimensions and font metadata
 */ function saveDimensions(sessionId, cols, rows, options) {
    if (true) return;
    try {
        const key = `terminal-dimensions-${sessionId}`;
        const payload = {
            cols,
            rows
        };
        // Validate dimensions before caching
        if (!isValidToCache(cols, rows)) {
            return;
        }
        // Check if these dimensions are better than existing cache
        if (!shouldOverwriteCache(key, cols, rows, options)) {
            return;
        }
        if (options?.cellWidth != null && options?.cellHeight != null) {
            payload.cellWidth = options.cellWidth;
            payload.cellHeight = options.cellHeight;
            payload.fontSize = options.fontSize;
            payload.fontFamily = options.fontFamily;
        }
        localStorage.setItem(key, JSON.stringify(payload));
        console.log(`[TerminalDimensionCache] Saved dimensions for ${sessionId}: ${cols}x${rows}${options?.cellWidth != null ? ` (cell: ${options.cellWidth.toFixed(2)}x${options.cellHeight.toFixed(2)})` : ''}`);
    } catch (err) {
        console.warn('[TerminalDimensionCache] Failed to save dimensions:', err);
    }
}
/**
 * Check if we should overwrite the existing cache entry.
 *
 * @param key - The localStorage key to check
 * @param cols - Number of terminal columns
 * @param rows - Number of terminal rows
 * @param options - Optional configuration for cell dimensions
 * @returns true if we should overwrite the cache, false otherwise
 */ function shouldOverwriteCache(key, cols, rows, options) {
    const existing = localStorage.getItem(key);
    if (!existing) return true;
    try {
        const existingDims = JSON.parse(existing);
        // Only overwrite if new dimensions are larger (better)
        if (existingDims.cols >= cols && existingDims.rows >= rows) {
            console.log(`[TerminalDimensionCache] Skipping smaller dimensions ${cols}x${rows} (existing: ${existingDims.cols}x${existingDims.rows})`);
            return false;
        }
        // If existing has cell dimensions and new doesn't, preserve existing
        if (existingDims.cellWidth != null && existingDims.cellHeight != null && (options?.cellWidth == null || options?.cellHeight == null)) {
            console.log(`[TerminalDimensionCache] Skipping dimensions without cell data (existing has cell: ${existingDims.cellWidth}x${existingDims.cellHeight})`);
            return false;
        }
    } catch (parseErr) {
        console.warn('[TerminalDimensionCache] Failed to parse existing dimensions:', parseErr);
        return false;
    }
    return true;
}

;// ./src/lib/terminal/preSizing.ts
/**
 * Pure helpers for TerminalOutput's pre-sizing: reading xterm's rendered cell
 * metrics and deriving cols/rows from a container's pixel size, so a connection
 * can start before xterm fires its first onResize.
 */ 
/**
 * Cell pixel metrics from xterm.js's private render service, or null when
 * unavailable (API changed, not rendered yet). isFinite() guards against
 * NaN/Infinity a corrupted render-service state could return.
 */ function readRenderedCellMetrics(terminal) {
    const cell = terminal?._core?._renderService?.dimensions?.css?.cell;
    if (cell?.width && cell?.height && isFinite(cell.width) && isFinite(cell.height)) {
        return {
            cellWidth: cell.width,
            cellHeight: cell.height
        };
    }
    return null;
}
/**
 * cols/rows that fit in a container of the given pixel size. Floors, so the
 * result never exceeds what actually fits.
 */ function computePreSize(container, cell) {
    if (!(container.width > 0 && container.height > 0)) {
        return {
            ok: false,
            reason: "zero-size"
        };
    }
    const cols = Math.floor(container.width / cell.cellWidth);
    const rows = Math.floor(container.height / cell.cellHeight);
    if (cols < MIN_COLS || rows < MIN_ROWS) {
        return {
            ok: false,
            reason: "below-minimum",
            cols,
            rows
        };
    }
    return {
        ok: true,
        cols,
        rows
    };
}

// EXTERNAL MODULE: ./src/lib/config/terminalConfig.ts
var terminalConfig = __webpack_require__(40731);
;// ./src/components/sessions/useTerminalSizing.ts
/* __next_internal_client_entry_do_not_use__ useTerminalResizeHandler,useSessionConnectionBootstrap auto */ /**
 * Size-stability and initial-connection logic for TerminalOutput.
 *
 * The refs are owned by the component (not this module) because useTerminalStream
 * is called with `lastResizeRef.current` as an input while its outputs
 * (`connect`, `isConnected`, `resize`) are what this logic consumes. Passing
 * them in keeps that cycle out of the hooks without changing useTerminalStream.
 */ 



/** Handles xterm's onResize: caches dimensions, drives size-stability connect, forwards resizes. */ function useTerminalResizeHandler({ sessionId, xtermRef, isConnected, error, connect, resize, clearBufferBeforeResize, setIsWaitingForStableSize, onTerminalResized, refs }) {
    const { lastResizeRef, hasInitiatedConnectionRef, hasCachedDimensionsRef, sizeStabilityTimeoutRef, isMountedRef, metricsRef, settleBypassPendingRef } = refs;
    return (0,react.useCallback)((cols, rows)=>{
        console.log(`[TerminalOutput] Terminal resized to ${cols}x${rows}`);
        onTerminalResized?.(cols, rows);
        const lastResize = lastResizeRef.current;
        const sizeChanged = !lastResize || lastResize.cols !== cols || lastResize.rows !== rows;
        if (sizeChanged) {
            lastResizeRef.current = {
                cols,
                rows
            };
            console.log(`[TerminalOutput] Saved resize dimensions: ${cols}x${rows}`);
            // Only persist dimensions that are plausibly real — transient tiny values
            // (e.g. 10x6) fired before the CSS container finishes layout would otherwise
            // corrupt the cache and cause the next session view to connect at the wrong size.
            if (cols >= MIN_COLS && rows >= MIN_ROWS) {
                // Also capture cell pixel dimensions so TerminalOutput can pre-calculate
                // cols/rows from the container size on the next mount, enabling an immediate
                // connection before xterm fires its first onResize event.
                const terminal = xtermRef.current?.terminal;
                saveDimensions(sessionId, cols, rows, {
                    ...readRenderedCellMetrics(terminal),
                    fontSize: terminal?.options?.fontSize ?? 14,
                    fontFamily: terminal?.options?.fontFamily ?? 'Menlo, Monaco, "Courier New", monospace'
                });
            } else {
                console.log(`[TerminalOutput] Skipping cache write for tiny dimensions ${cols}x${rows} (below ${MIN_COLS}x${MIN_ROWS})`);
            }
            if (metricsRef.current.firstResizeTime === null) {
                metricsRef.current.firstResizeTime = performance.now();
            }
            metricsRef.current.resizeCount++;
            // Skip size stability wait if we have cached dimensions, but only when
            // the cached size (lastResize) is itself reasonable — a stale tiny cache
            // entry would otherwise bypass the stability wait and connect at the wrong size.
            if (hasCachedDimensionsRef.current && !hasInitiatedConnectionRef.current && !isConnected && !error && isMountedRef.current) {
                const initDims = {
                    cols,
                    rows
                };
                if (initDims.cols >= MIN_COLS && initDims.rows >= MIN_ROWS) {
                    console.log(`[TerminalOutput] Using cached dimensions, skipping stability wait (${initDims.cols}x${initDims.rows})`);
                    metricsRef.current.sizeStableTime = performance.now();
                    metricsRef.current.connectionInitTime = performance.now();
                    hasInitiatedConnectionRef.current = true;
                    setIsWaitingForStableSize(false);
                    connect(initDims.cols, initDims.rows);
                    return;
                } else {
                    // Cached value is too small — treat as no cache and wait for stable size.
                    console.log(`[TerminalOutput] Cached dimensions ${initDims.cols}x${initDims.rows} too small, falling through to stability wait`);
                    hasCachedDimensionsRef.current = false; // prevents future onResize events from fast-connecting on stale cache
                }
            }
            // Event-driven size stability detection for initial connection
            if (!hasInitiatedConnectionRef.current && !isConnected && !error && isMountedRef.current) {
                if (sizeStabilityTimeoutRef.current) {
                    clearTimeout(sizeStabilityTimeoutRef.current);
                }
                console.log(`[TerminalOutput] Size changed, waiting for layout to stabilize...`);
                setIsWaitingForStableSize(true);
                sizeStabilityTimeoutRef.current = setTimeout(()=>{
                    requestAnimationFrame(()=>{
                        requestAnimationFrame(()=>{
                            if (!hasInitiatedConnectionRef.current && !isConnected && isMountedRef.current) {
                                const stableSize = lastResizeRef.current;
                                if (stableSize) {
                                    metricsRef.current.sizeStableTime = performance.now();
                                    metricsRef.current.connectionInitTime = performance.now();
                                    console.log(`[TerminalOutput] Layout stable at ${stableSize.cols}x${stableSize.rows}, initiating connection`);
                                    hasInitiatedConnectionRef.current = true;
                                    setIsWaitingForStableSize(false);
                                    connect(stableSize.cols, stableSize.rows);
                                }
                            }
                        });
                    });
                    sizeStabilityTimeoutRef.current = null;
                }, 50);
            }
        }
        if (!isConnected) {
            console.log(`[TerminalOutput] Resize blocked - not connected (${cols}x${rows})`);
            return;
        }
        if (!sizeChanged) {
            console.log(`[TerminalOutput] Resize blocked - unchanged (${cols}x${rows})`);
            return;
        }
        console.log(`[TerminalOutput] Sending resize: ${cols}x${rows} (prev: ${lastResize?.cols || 'none'}x${lastResize?.rows || 'none'})`);
        clearBufferBeforeResize();
        if (settleBypassPendingRef?.current) {
            settleBypassPendingRef.current = false;
            resize(cols, rows, false, {
                bypassBounceHold: true
            });
        } else {
            resize(cols, rows);
        }
    }, [
        isConnected,
        resize,
        connect,
        error,
        sessionId,
        clearBufferBeforeResize,
        onTerminalResized,
        xtermRef
    ]);
}
/**
 * The three effects that start a connection for a session, in a fixed order:
 * pre-size from cache, session-switch connect, then post-disconnect connect.
 */ function useSessionConnectionBootstrap({ sessionId, containerWidth, terminalContainerRef, xtermRef, isConnected, connect, setIsWaitingForStableSize, setIsLoadingInitialContent, setConnectionAttempts, setShowReconnectButton, reconnectTimeoutRef, previousConnectionStateRef, streamManagerRef, refs }) {
    const { lastResizeRef, hasInitiatedConnectionRef, hasCachedDimensionsRef, pendingConnectAfterDisconnectRef, isMountedRef, metricsRef } = refs;
    // Initialize with cached dimensions on mount.
    // When cell pixel metrics are also cached, pre-calculate cols/rows from the
    // container's current pixel size so the session-switch effect can connect
    // immediately — before xterm.js fires its first onResize event.
    //
    // ORDERING INVARIANT: This effect MUST remain defined before the session-switch
    // effect below (both share the [sessionId] dependency). React runs same-dependency
    // effects in definition order, so this effect runs first and populates
    // lastResizeRef before the session-switch effect reads it to trigger connect().
    // Moving this effect below the session-switch effect will silently break pre-sizing.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const rawCached = getCachedDimensions(sessionId);
        // Validate cell dims against current font config (R1.6): stale dims from a different
        // font configuration produce an incorrect initial fit() and wrong initial resize.
        // loadTerminalConfig() is what the pooled XtermTerminal renders with (useConfig), so
        // validating against it keeps cached cell dims in sync with the actual font (Bug 4 fix).
        const { fontSize: currentFontSize, fontFamily: currentFontFamily } = (0,terminalConfig/* loadTerminalConfig */.X6)();
        const cached = rawCached ? validateCellDimensions(rawCached, currentFontSize, currentFontFamily) : null;
        if (cached && cached.cols >= MIN_COLS && cached.rows >= MIN_ROWS) {
            hasCachedDimensionsRef.current = true;
            console.log(`[TerminalOutput] Initialized with cached dimensions: ${cached.cols}x${cached.rows}`);
            if (cached.cellWidth && cached.cellHeight && terminalContainerRef.current) {
                // Guard: only use getBoundingClientRect after the ResizeObserver has confirmed
                // the container has a real constrained width (not the full pre-layout browser width).
                // containerWidth > 0 means layout is complete and the pane has its actual dimensions.
                if (containerWidth > 0) {
                    const rect = terminalContainerRef.current.getBoundingClientRect();
                    const preSize = computePreSize(rect, {
                        cellWidth: cached.cellWidth,
                        cellHeight: cached.cellHeight
                    });
                    if (preSize.ok) {
                        const { cols: preCols, rows: preRows } = preSize;
                        console.log(`[TerminalOutput] Pre-sizing: ${rect.width}×${rect.height}px / ` + `${cached.cellWidth.toFixed(2)}×${cached.cellHeight.toFixed(2)}px/cell → ${preCols}×${preRows}`);
                        lastResizeRef.current = {
                            cols: preCols,
                            rows: preRows
                        };
                        // If this effect fired because containerWidth became non-zero (i.e. ResizeObserver
                        // fired after the initial render), the session-switch effect won't re-run because
                        // sessionId didn't change. We must initiate the connection here in that case.
                        // On a sessionId-triggered render, session-switch runs after this and connects,
                        // so we only act when no connection has been initiated yet.
                        const isXtermDefault = preCols === XTERM_DEFAULT_COLS && preRows === XTERM_DEFAULT_ROWS;
                        if (!hasInitiatedConnectionRef.current && !isConnected && isMountedRef.current && !isXtermDefault) {
                            hasInitiatedConnectionRef.current = true;
                            setIsWaitingForStableSize(false);
                            // Grow the xterm buffer to preCols/preRows BEFORE connecting — otherwise the
                            // terminal is still at its 80x24 constructor default and the capture-pane
                            // snapshot's cursor-positioning sequences for rows beyond 24 are silently
                            // dropped, leaving them unpainted until a later resize forces a full repaint.
                            xtermRef.current?.resize(preCols, preRows);
                            connect(preCols, preRows);
                        }
                    } else if (preSize.reason === "below-minimum") {
                        console.log(`[TerminalOutput] Pre-sizing skipped: calculated ${preSize.cols}x${preSize.rows} below minimum`);
                    } else {
                        console.log(`[TerminalOutput] Pre-sizing skipped: container has zero size`);
                    }
                } else {
                    console.log(`[TerminalOutput] Pre-sizing deferred: container layout not yet resolved (containerSize.width=0)`);
                }
            }
        } else if (cached) {
            console.log(`[TerminalOutput] Ignoring stale cached dimensions ${cached.cols}x${cached.rows} (below ${MIN_COLS}x${MIN_ROWS})`);
        }
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        sessionId,
        containerWidth
    ]);
    // Reset loading state when switching sessions and trigger reconnect
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        setIsLoadingInitialContent(true);
        hasInitiatedConnectionRef.current = false;
        metricsRef.current.mountTime = performance.now();
        metricsRef.current.firstOutputTime = null;
        // Reset connection tracking so the new session doesn't inherit stale state
        previousConnectionStateRef.current = false;
        setConnectionAttempts(0);
        setShowReconnectButton(false);
        if (reconnectTimeoutRef.current) {
            clearTimeout(reconnectTimeoutRef.current);
            reconnectTimeoutRef.current = null;
        }
        // Reset stream manager for new session
        if (streamManagerRef.current) {
            streamManagerRef.current.cleanup();
            streamManagerRef.current = null;
        }
        // Connect immediately if already disconnected (e.g. first load, or was already disconnected)
        // Otherwise set the pending flag so we connect once the in-progress disconnect resolves
        if (!isConnected) {
            const dims = lastResizeRef.current;
            // Guard: don't fast-connect with xterm default dims (80×24). Those are the
            // terminal's initial values before fitAddon.fit() measures the container —
            // if Bug 1 corrupted the cache to 80×24, using them here would cause a thin
            // PTY. The resize handler will fire with the actual dims and connect normally.
            const isXtermDefault = dims?.cols === XTERM_DEFAULT_COLS && dims?.rows === XTERM_DEFAULT_ROWS;
            if (dims && isMountedRef.current && !isXtermDefault) {
                hasInitiatedConnectionRef.current = true;
                setIsWaitingForStableSize(false);
                connect(dims.cols, dims.rows);
            }
        // If no dims yet (or dims are xterm defaults), resize handler will fire and trigger connect normally
        } else {
            // Was connected to previous session — disconnect() is in-flight (async).
            // Mark pending so the isConnected→false transition triggers connect below.
            pendingConnectAfterDisconnectRef.current = true;
        }
        // Safety net: if the container is hidden at mount (display:none, 0×0), the
        // ResizeObserver zero-size guard prevents fitAddon.fit(), so no resize event
        // fires and the stability timer never starts. After 5s, attempt to connect
        // with whatever valid cached dims are available, or skip silently.
        const safetyTimeout = setTimeout(()=>{
            if (!hasInitiatedConnectionRef.current && isMountedRef.current) {
                const dims = lastResizeRef.current;
                if (dims && dims.cols >= MIN_COLS && dims.rows >= MIN_ROWS) {
                    console.log(`[TerminalOutput] Safety timeout: connecting with cached dims ${dims.cols}x${dims.rows} (container may have been hidden at mount)`);
                    hasInitiatedConnectionRef.current = true;
                    setIsWaitingForStableSize(false);
                    connect(dims.cols, dims.rows);
                } else {
                    console.log(`[TerminalOutput] Safety timeout: no valid dims available, container still not visible`);
                }
            }
        }, 5000);
        return ()=>{
            clearTimeout(safetyTimeout);
            setIsLoadingInitialContent(false);
        };
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        sessionId
    ]);
    // When a session switch happened while connected, the previous disconnect() is async (up to 1s).
    // This effect fires once isConnected transitions to false, completing the switch.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!isConnected && pendingConnectAfterDisconnectRef.current && !hasInitiatedConnectionRef.current && isMountedRef.current) {
            pendingConnectAfterDisconnectRef.current = false;
            const dims = lastResizeRef.current;
            if (dims) {
                console.log(`[TerminalOutput] Post-disconnect connect for new session: ${dims.cols}x${dims.rows}`);
                hasInitiatedConnectionRef.current = true;
                setIsWaitingForStableSize(false);
                connect(dims.cols, dims.rows);
            }
        // If no dims, the resize handler will fire and connect normally
        }
    }, [
        isConnected,
        connect
    ]);
}

;// ./src/components/sessions/useBlockedToast.ts
/* __next_internal_client_entry_do_not_use__ useBlockedToast auto */ 


const CONNECTION_PULSE_MS = 600;
/**
 * Story 1.4.3 — blocked-outcome toast plus the ConnectionCountIndicator pulse.
 * Owns its timers and clears them on unmount.
 */ function useBlockedToast() {
    const [blockedToast, setBlockedToast] = (0,react.useState)(null);
    const seqRef = (0,react.useRef)(0);
    const toastTimerRef = (0,react.useRef)(null);
    const [connectionPulse, setConnectionPulse] = (0,react.useState)(false);
    const pulseTimerRef = (0,react.useRef)(null);
    const dismissBlockedToast = (0,react.useCallback)(()=>{
        if (toastTimerRef.current) {
            clearTimeout(toastTimerRef.current);
            toastTimerRef.current = null;
        }
        setBlockedToast(null);
    }, []);
    // Keyed on blocked_reason. UNSPECIFIED (a gate failure the client should never
    // actually attempt a request for, per Epic 1.1's eligibility check) falls back
    // to the MULTIPLE_VIEWERS copy as the least-wrong default rather than showing no
    // message — a deliberate P2 tradeoff (pre-mortem.md Failure #2), accepted since
    // UNSPECIFIED should be unreachable in practice.
    const showBlockedOutcome = (0,react.useCallback)((frame)=>{
        seqRef.current += 1;
        setBlockedToast({
            reason: frame.blockedReason,
            program: frame.program,
            seq: seqRef.current
        });
        if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
        toastTimerRef.current = setTimeout(()=>setBlockedToast(null), notification_policy/* DEFAULT_TOAST_MS */.tx);
        // Pulse ConnectionCountIndicator only for the reason that actually has
        // connected-viewer state to point to (and its UNSPECIFIED fallback, which
        // renders the same copy).
        if (frame.blockedReason === events_pb/* ScrollBlockedReason */.ec.MULTIPLE_VIEWERS || frame.blockedReason === events_pb/* ScrollBlockedReason */.ec.SCROLL_BLOCKED_REASON_UNSPECIFIED) {
            if (pulseTimerRef.current) clearTimeout(pulseTimerRef.current);
            setConnectionPulse(true);
            pulseTimerRef.current = setTimeout(()=>setConnectionPulse(false), CONNECTION_PULSE_MS);
        }
    }, []);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        return ()=>{
            if (toastTimerRef.current) clearTimeout(toastTimerRef.current);
            if (pulseTimerRef.current) clearTimeout(pulseTimerRef.current);
        };
    }, []);
    return {
        blockedToast,
        connectionPulse,
        showBlockedOutcome,
        dismissBlockedToast
    };
}

;// ./src/lib/terminal/bufferFilter.ts
/**
 * Display-only filtering of terminal output. Works on lines read out of the xterm
 * buffer, never on the PTY byte stream, so filtering can't corrupt rendering.
 */ const MAX_FILTER_RESULTS = 500;
/** Lines of the active xterm buffer (scrollback + viewport) as plain text. */ function readBufferLines(terminal) {
    if (!terminal) return [];
    const buf = terminal.buffer.active;
    const lines = [];
    for(let i = 0; i < buf.length; i++){
        lines.push(buf.getLine(i)?.translateToString(true) ?? "");
    }
    return lines;
}
function buildMatcher(query, { regex, caseSensitive }) {
    if (regex) {
        try {
            const re = new RegExp(query, caseSensitive ? "" : "i");
            return (s)=>re.test(s);
        } catch (err) {
            return {
                error: err instanceof Error ? err.message : "Invalid pattern"
            };
        }
    }
    const needle = caseSensitive ? query : query.toLowerCase();
    return (s)=>(caseSensitive ? s : s.toLowerCase()).includes(needle);
}
/** Matching lines for `query`; an empty query matches nothing (the panel shows a hint instead). */ function filterLines(lines, query, options = {}) {
    if (query === "") return {
        matches: [],
        truncated: false
    };
    const matcher = buildMatcher(query, options);
    if ("error" in matcher) return {
        matches: [],
        truncated: false,
        error: matcher.error
    };
    const all = [];
    lines.forEach((text, i)=>{
        if (matcher(text)) all.push({
            line: i + 1,
            text
        });
    });
    const truncated = all.length > MAX_FILTER_RESULTS;
    return {
        matches: truncated ? all.slice(-MAX_FILTER_RESULTS) : all,
        truncated
    };
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FTerminalOutputFilter.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA61Ty27jMAy85yt4KbA5qIjdTYuqH7OQLdomIouCpNSPov--kJwGgZMCLdqTaFLmcIaj-zBWs2938LYBcBwoElsJqgpsjhFfNgCRnYRdigw28RR6aruPeBZkNY4Sysf0qSk4oyYJjcExJdIpNHmsl-Y1m2NvU6VXo-hwabXf3aVUpepD6_lotYRX5f8I8a94xrn089M219lr9KLiGLmXULgRAhvSq8u4fdm8b-4XdkVmdzWXMtRaQRH7IKFGG9GndKvcGrqcM7ZTWpNtb1bPYGUGSxgSikySrBhIx-4k12ddpu1t1IZtFIFmXP_xUF3iPvyY5PQFOEjbY7-uFbk0dBRRBKdqlGB58MpdDPh38dgH-901218e6Iy8z8i98i3Z9RIWY1OIIsTJ5Llttj2_om8MD2KSoI6Rz6M0qiczSejZcub6jSU93l7SFyx3Q64ryZ1HsYgOMLDXovKoDhLyIZQxF5M8nW0qQufJHk5SXLh1X3f5_eMYRbaRXJ79p6KXiep_d1o0FlIEAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTerminalOutputFilter_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA61Ty27jMAy85yt4KbA5qIjdTYuqH7OQLdomIouCpNSPov_kJwGgZMCLdqTaFLmcIaj_zBWs2938LYBcBwoElsJqgpsjhFfNgCRnYRdigw28RR6aruPeBZkNY4Sysf0qSk4oyYJjcExJdIpNHmsl_Y1m2NvU6VXo_hwabXf3aVUpepD6_lotYRX5f8I8a94xrn089M219lr9KLiGLmXULgRAhvSq8u4fdm8b_4XdkVmdzWXMtRaQRH7IKFGG9GndKvcGrqcM7ZTWpNtb1bPYGUGSxgSikySrBhIx_4k12ddpu1t1IZtFIFmXP_xUF3iPvyY5PQFOEjbY7_uFbk0dBRRBKdqlGB58MpdDPh38dgH_901218e6Iy8z8i98i3Z9RIWY1OIIsTJ5Llttj2_om8MD2KSoI6Rz6M0qiczSejZcub6jSU93l7SFyx3Q64ryZ1HsYgOMLDXovKoDhLyIZQxF5M8nW0qQufJHk5SXLh1X3f5_eMYRbaRXJ79p6KXiep_d1o0FlIEAAA_22_7D = __webpack_require__(65176);
;// ./src/components/sessions/TerminalOutputFilter.css.ts


var header = 'sxbzrg1';
var input = 'sxbzrg2';
var lineNumber = 'sxbzrg7';
var TerminalOutputFilter_css_option = 'sxbzrg3';
var panel = 'sxbzrg0';
var results = 'sxbzrg5';
var row = 'sxbzrg6';
var TerminalOutputFilter_css_status = 'sxbzrg4';
;// ./src/components/sessions/TerminalOutputFilter.tsx
/* __next_internal_client_entry_do_not_use__ TerminalOutputFilter auto */ 
// +feature: terminal-output-filter



const REFRESH_MS = 1000;
/**
 * Opt-in, display-only filter over the terminal's buffer. It renders its own list
 * and never touches the PTY stream or xterm's buffer.
 */ function TerminalOutputFilter({ getLines, onClose }) {
    const [query, setQuery] = (0,react.useState)("");
    const [regex, setRegex] = (0,react.useState)(false);
    const [caseSensitive, setCaseSensitive] = (0,react.useState)(false);
    const [result, setResult] = (0,react.useState)({
        matches: [],
        truncated: false
    });
    const inputRef = (0,react.useRef)(null);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        inputRef.current?.focus();
    }, []);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const run = ()=>setResult(filterLines(getLines(), query, {
                regex,
                caseSensitive
            }));
        run();
        const id = setInterval(run, REFRESH_MS);
        return ()=>clearInterval(id);
    }, [
        getLines,
        query,
        regex,
        caseSensitive
    ]);
    const hasQuery = query !== "";
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: panel,
        role: "search",
        "aria-label": "Filter terminal output",
        "data-testid": "terminal-output-filter",
        onKeyDown: (e)=>{
            if (e.key === "Escape") onClose();
        },
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: header,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                        ref: inputRef,
                        className: input,
                        type: "text",
                        value: query,
                        onChange: (e)=>setQuery(e.target.value),
                        placeholder: "Filter lines…",
                        "aria-label": "Filter text",
                        "data-testid": "terminal-filter-input"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                        className: TerminalOutputFilter_css_option,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                type: "checkbox",
                                checked: regex,
                                onChange: (e)=>setRegex(e.target.checked),
                                "data-testid": "terminal-filter-regex"
                            }),
                            "Regex"
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                        className: TerminalOutputFilter_css_option,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                type: "checkbox",
                                checked: caseSensitive,
                                onChange: (e)=>setCaseSensitive(e.target.checked),
                                "data-testid": "terminal-filter-case"
                            }),
                            "Case"
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                        type: "button",
                        onClick: onClose,
                        "aria-label": "Close filter",
                        "data-testid": "terminal-filter-close",
                        children: "✕"
                    })
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                className: TerminalOutputFilter_css_status,
                role: "status",
                "data-testid": "terminal-filter-status",
                children: result.error ? `Invalid pattern: ${result.error}` : !hasQuery ? "Type to filter the terminal's scrollback. Display only — the terminal is unchanged." : result.truncated ? `Showing the last ${MAX_FILTER_RESULTS} matching lines` : `${result.matches.length} matching ${result.matches.length === 1 ? "line" : "lines"}`
            }),
            result.matches.length > 0 && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("ul", {
                className: results,
                "data-testid": "terminal-filter-results",
                children: result.matches.map((m)=>/*#__PURE__*/ (0,react_jsx_runtime.jsxs)("li", {
                        className: row,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: lineNumber,
                                children: m.line
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                children: m.text
                            })
                        ]
                    }, m.line))
            })
        ]
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FTerminalOutput.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA-0aW4-cuvk9v8JVFWVGGkbA3DlSFanPeazUqqqOPGBmvAs2MWZuVf57ZWPABgPDbpJzUZVks9gfn7_7DX9-RfeYwRTl4FcfHvwQueC_HwBwPy6A57of5QMANIMh5vcAeL98AODbBwA23T13uSl3v3343EHrKbTqJc4gyWPK0gAwyiFHMzdCp3mFXTu6C7naarDWs7Y_7LClOsKXL0Q4zxJ4D0CcoJsAEf87EWYo5JiSAIQ0KVIids4In848kIeJ5xQTp1pzq1eVfI8wfD0xWpAoABfIZo7zq3dAD589vEhSEtKEss4eknsxJdyJYYqTewA-faEEhvTTAnz6gkgif_nHsSC8AF8okY9_pySnCczF7zktWIickEbIyZjYTimheQZD9IvG_MrOPEzwiTiYozQPQIgIR0wsvxQ5x_HdCSnhiPAASHzOEfErQlI2GYwiTE7CgnYbhlLgMZQOC2IneT1SFiHmHCnnNA2Al91AThMctYCVYIRu8jPD5FWKvOFnPYmfE8ykrUsaGyQbieSKI34OwD676UpXj4pcBiNc5IFwIXkMwSkszaV2QT8HmMSYYG4IvrTrfqkUSiqCTxjRawBc4IrTLXAN1t0IVs99Eq0CrBlyCExRAAglBhf7kfNuTx53m_eJz13u7QI8yKOli-T4gYQe97uNsja7Vxmigu33Pb9UrVy6KnVvXbcXXzo3LN7PbrV19CvAsPbKfNa6UfUbf2wwcJwugFKpKWQnTJwExdxm_aHdhXp8JZLQmttLr1fePyyL_fwZptGAwKxcuk3stMilYLl4JaO4CgIyUeDS7Bpigbv080V1rjxILhm8B2d6QWzEB446A1aK8dzECkOOL2gE7c546WQqTQVog1ps09TvRUNDChL5VQXjEo9Y4LQIzw6H7IT4AqzXWRlC9Fw8CvzmtNds9Wci_EOM4-UJ51RUXZlYFD91BMnbE2RtPK3QUdNfLg-ZSUNH-j0SNZkUqqgJ3c5l2aiqwF9wmlHGIeHjWjOAm1O-2okIGKX83xHkUErROUMSoeg_wHytXZMyejVx_w0cC84paaA12wTgesYcObJoE0e3bYOZtGGSYIKcQdU0L-djpcduivzSNrAV6muflPnviZjCDLzjFbHnTSmJVVYfyvjWZNgQeKnNRXUtnY6mLBnUQ5NC3t3hiAgZJ6IuPOMoUi0ErbIxQwkUmbAbekpkOYyRAxkqfWYBXBXXK1im54EGWC5X0I0UrqWa6tPhMadJweXpmOSoksXDwSQSovLd75REGhFW9sdORzhzF0D9Xa43JWNlUnTQRST3bgi7SRYUjr_GcdxSfmV6thK31qqozr2ttc3ZmnWqqHgjmJ9RVBLsbzYL0Pxwl6rY1SykzdhKQhQ5Yk6OEhTyLlP3Ab1wmimtqJQkfmWa4VbOM2Kq0F4P9M8ApitZJqVKAbUNeTVhEaOZE-OEC8kek4LN_JaBPvS2dN3qS9d7UzfrvmgR67FFyq8PtLDVdb2drge8HIiEAZm1X_Pc6f2KNgaprHXjGlWW5_2JrKMpqAz70Lj12zL0h-TnG-bjrTod71i3bIsUGsK1RaOeP6jSFkmbJ2PueAj5DmHYHl01ZWwM2ne6O6580x2r58odV8-7Yx9oOs0dt-XoZMgj91Mnn8pK_VY_INbWZdvf7hQqD0vhbdbOw-VemYgXCkUnYbTEhTvi6i2L4pFRoXcY7B5WipNeg9EwwXYBpQtGDURqoWntrPtbtKwD0n19qt83bbbS8HMWXtvtwHjANv3G5IwY5t1i1lbRbNTQrjtKsBQaADhXdHzF3LFulnqAyhVSSHBWJNLV5C66cUeqQJewtc0aGDS5-2bSVIqzNJBqq_6UUS6YpvfctMhQrXGI1HNNXvnBRP6aQI7-1SlCvOPYSd6U1spzn-qtPL8NZih856p0bs6ZXWnClo5IYyccqx9a-esxb-oJFXxtgvvnzNm4H-eWfqEODS3E93lrZeXZHOdwOBzGhsyH_r7L1g82KXuSOY81I170s0U7aZpkPfwnKMdb9WtnM6idkemNF-nDH50AKboMMqT7Y39830yd5256P4RYQrCdOwsbPfPTutOs_rnLVlGJ_m93v1FQ0JQQv9cK3alW6L7ZCusutB3SYMGpwdXpj2Ra9z-TaT2Rb84t5cT4hiK9zW_x679dOZ6737xTPavqY-ytqsNSTGYH93JdgLXfGmVaDOsp7bVguvM4E-Der8jHoCLlh4L6Xsxy3a3GlDtW3bv80w7cuOezxTuiyGNqFHm8N4po_Lz8vGjhbfpr55eBz61v9LULWN44YqlzwegqqvIgqLqoPGQ0SY6Q6eMRrzUeKZ-n4XM4g-HrWPcRzafjPRfpaFdj7WIt83BrC_xWop76hCxbu4nYQ8rIOOIesssWGVsKTctdF8v7Z5RkYnqDblzMgTpTyW12a31J-5yiCEOQhwwhAiCJwEyLm7vtPrvNJRrzWtvgvZRvGvhBgZs-YgEMFWB7ENZ8yXJuVdkgVivJ17ulCiRJcrJQgvV4IQCNxhSv1VgCACfNNbT8nqBmVyc56opCzeq2NfHWkcrEydS0exo6hSdFYeujuAn08seQvEjo-auDU3hCgRq-OicRLxDhM06B-vh4TEQsC2ESzuSdVUdOjOcLPb_JC6Zzk50fgV0X8-szukieASLPANE2UFVH9VZSg6PNaTY42cB7jEVn6Gs_Q5bLGw5DF8RypAHot3bs9q458kqLacqvS5ix27YT79vaijKda6a4_kkB49IJaYZknrojMO2WgLqePpiG1nu3nYZ6Ym-bn7jfVepDZ6okE20NQeYx54H3_wdbXvJJFjAAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FTerminalOutput_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA_0aW4_cuvk9v8JVFWVGGkbA3DlSFanPeazUqqqOPGBmvAs2MWZuVf57ZWPABgPDbpJzUZVks9gfn7_7DX9_RfeYwRTl4FcfHvwQueC_HwBwPy6A57of5QMANIMh5vcAeL98AODbBwA23T13uSl3v3343EHrKbTqJc4gyWPK0gAwyiFHMzdCp3mFXTu6C7naarDWs7Y_7LClOsKXL0Q4zxJ4D0CcoJsAEf87EWYo5JiSAIQ0KVIids4In848kIeJ5xQTp1pzq1eVfI8wfD0xWpAoABfIZo7zq3dAD589vEhSEtKEss4eknsxJdyJYYqTewA_faEEhvTTAnz6gkgif_nHsSC8AF8okY9_pySnCczF7zktWIickEbIyZjYTimheQZD9IvG_MrOPEzwiTiYozQPQIgIR0wsvxQ5x_HdCSnhiPAASHzOEfErQlI2GYwiTE7CgnYbhlLgMZQOC2IneT1SFiHmHCnnNA2Al91AThMctYCVYIRu8jPD5FWKvOFnPYmfE8ykrUsaGyQbieSKI34OwD676UpXj4pcBiNc5IFwIXkMwSkszaV2QT8HmMSYYG4IvrTrfqkUSiqCTxjRawBc4IrTLXAN1t0IVs99Eq0CrBlyCExRAAglBhf7kfNuTx53m_eJz13u7QI8yKOli_T4gYQe97uNsja7Vxmigu33Pb9UrVy6KnVvXbcXXzo3LN7PbrV19CvAsPbKfNa6UfUbf2wwcJwugFKpKWQnTJwExdxm_aHdhXp8JZLQmttLr1fePyyL_fwZptGAwKxcuk3stMilYLl4JaO4CgIyUeDS7Bpigbv080V1rjxILhm8B2d6QWzEB446A1aK8dzECkOOL2gE7c546WQqTQVog1ps09TvRUNDChL5VQXjEo9Y4LQIzw6H7IT4AqzXWRlC9Fw8CvzmtNds9Wci_EOM4_UJ51RUXZlYFD91BMnbE2RtPK3QUdNfLg_ZSUNH_j0SNZkUqqgJ3c5l2aiqwF9wmlHGIeHjWjOAm1O_2okIGKX83xHkUErROUMSoeg_wHytXZMyejVx_w0cC84paaA12wTgesYcObJoE0e3bYOZtGGSYIKcQdU0L_djpcduivzSNrAV6muflPnviZjCDLzjFbHnTSmJVVYfyvjWZNgQeKnNRXUtnY6mLBnUQ5NC3t3hiAgZJ6IuPOMoUi0ErbIxQwkUmbAbekpkOYyRAxkqfWYBXBXXK1im54EGWC5X0I0UrqWa6tPhMadJweXpmOSoksXDwSQSovLd75REGhFW9sdORzhzF0D9Xa43JWNlUnTQRST3bgi7SRYUjr_GcdxSfmV6thK31qqozr2ttc3ZmnWqqHgjmJ9RVBLsbzYL0Pxwl6rY1SykzdhKQhQ5Yk6OEhTyLlP3Ab1wmimtqJQkfmWa4VbOM2Kq0F4P9M8ApitZJqVKAbUNeTVhEaOZE_OEC8kek4LN_JaBPvS2dN3qS9d7UzfrvmgR67FFyq8PtLDVdb2drge8HIiEAZm1X_Pc6f2KNgaprHXjGlWW5_2JrKMpqAz70Lj12zL0h_TnG_bjrTod71i3bIsUGsK1RaOeP6jSFkmbJ2PueAj5DmHYHl01ZWwM2ne6O6580x2r58odV8_7Yx9oOs0dt_XoZMgj91Mnn8pK_VY_INbWZdvf7hQqD0vhbdbOw_VemYgXCkUnYbTEhTvi6i2L4pFRoXcY7B5WipNeg9EwwXYBpQtGDURqoWntrPtbtKwD0n19qt83bbbS8HMWXtvtwHjANv3G5IwY5t1i1lbRbNTQrjtKsBQaADhXdHzF3LFulnqAyhVSSHBWJNLV5C66cUeqQJewtc0aGDS5_2bSVIqzNJBqq_6UUS6YpvfctMhQrXGI1HNNXvnBRP6aQI7_1SlCvOPYSd6U1spzn_qtPL8NZih856p0bs6ZXWnClo5IYyccqx9a_esxb_oJFXxtgvvnzNm4H_eWfqEODS3E93lrZeXZHOdwOBzGhsyH_r7L1g82KXuSOY81I170s0U7aZpkPfwnKMdb9WtnM6idkemNF_nDH50AKboMMqT7Y39830yd5256P4RYQrCdOwsbPfPTutOs_rnLVlGJ_m93v1FQ0JQQv9cK3alW6L7ZCusutB3SYMGpwdXpj2Ra9z_TaT2Rb84t5cT4hiK9zW_x679dOZ6737xTPavqY_ytqsNSTGYH93JdgLXfGmVaDOsp7bVguvM4E_Der8jHoCLlh4L6Xsxy3a3GlDtW3bv80w7cuOezxTuiyGNqFHm8N4po_Lz8vGjhbfpr55eBz61v9LULWN44YqlzwegqqvIgqLqoPGQ0SY6Q6eMRrzUeKZ_n4XM4g_HrWPcRzafjPRfpaFdj7WIt83BrC_xWop76hCxbu4nYQ8rIOOIesssWGVsKTctdF8v7Z5RkYnqDblzMgTpTyW12a31J_5yiCEOQhwwhAiCJwEyLm7vtPrvNJRrzWtvgvZRvGvhBgZs_YgEMFWB7ENZ8yXJuVdkgVivJ17ulCiRJcrJQgvV4IQCNxhSv1VgCACfNNbT8nqBmVyc56opCzeq2NfHWkcrEydS0exo6hSdFYeujuAn08seQvEjo_auDU3hCgRq_OicRLxDhM06B_vh4TEQsC2ESzuSdVUdOjOcLPb_JC6Zzk50fgV0X8_szukieASLPANE2UFVH9VZSg6PNaTY42cB7jEVn6Gs_Q5bLGw5DF8RypAHot3bs9q458kqLacqvS5ix27YT79vaijKda6a4_kkB49IJaYZknrojMO2WgLqePpiG1nu3nYZ6Ym_bn7jfVepDZ6okE20NQeYx54H3_wdbXvJJFjAAAA_22_7D = __webpack_require__(29953);
;// ./src/components/sessions/TerminalOutput.css.ts


var actions = '_2a92cec';
var TerminalOutput_css_blockedToast = '_2a92ce1h';
var blockedToastButton = '_2a92ce1i';
var connected = '_2a92ce6';
var container = '_2a92ce2';
var debugActive = '_2a92cee';
var desktopOnlyUploads = '_2a92ceg';
var devGroup = '_2a92cem';
var devGroupPanel = '_2a92cel';
var devOnly = '_2a92cek';
var disconnected = '_2a92ce7';
var dropZoneLabel = '_2a92cex';
var dropZoneOverlay = '_2a92cew';
var error = '_2a92ceu';
var errorText = '_2a92ceb';
var externalLabel = '_2a92cea';
var hardFailedBanner = '_2a92ce1d';
var loadingOverlay = '_2a92cey';
var loadingSpinner = '_2a92cez';
var loadingText = '_2a92ce10';
var mobileKey = '_2a92ce1a';
var mobileKeyActive = '_2a92ces';
var mobileKeyCtrlC = '_2a92ce1b';
var mobileKeyRow = '_2a92ce19';
var mobileKeyboard = '_2a92ce18';
var mobileKeyboardToggle = '_2a92cer';
var mobileMoreActive = '_2a92cep';
var mobileMoreButton = '_2a92ceo';
var mobileOnlyUpload = '_2a92ceh';
var mobileOverflowRow = '_2a92ceq';
var mouseModeActive = '_2a92cet';
var noMoreHistoryLine = '_2a92ce1j';
var reconnectingBanner = '_2a92ce1c';
var resizingOverlay = '_2a92ce15';
var resizingSpinner = '_2a92ce17';
var scrollLoadingPill = '_2a92ce1e';
var scrollLoadingPillCancel = '_2a92ce1f';
var scrollSourceIndicator = '_2a92ce1g';
var secondaryGroup = '_2a92cen';
var stabilizing = '_2a92ce8';
var TerminalOutput_css_status = '_2a92ce4';
var statusIndicator = '_2a92ce5';
var statusText = '_2a92ce9';
var terminal = '_2a92cev';
var toolbar = '_2a92ce3';
var toolbarActions = '_2a92cej';
var toolbarButton = '_2a92ced';
var toolbarButtonLabel = '_2a92cef';
var toolbarToggle = '_2a92cei';
var unavailableIcon = '_2a92ce12';
var unavailableOverlay = '_2a92ce11';
var unavailableSubtext = '_2a92ce14';
var unavailableText = '_2a92ce13';
;// ./src/components/sessions/ScrollLoadingPill.tsx
/* __next_internal_client_entry_do_not_use__ ScrollLoadingPill auto */ 
// +feature: scroll-forward-loading-pill

/**
 * Loading affordance for a scroll-up-triggered scrollback fetch (Story 1.4.5,
 * design/ux.md Surface 1) — shared by both the pre-existing tmux-native
 * paged-scrollback path and Story 1.4.0's app-forwarded (alt-screen) path.
 * Reuses `reconnectingBanner`'s exact visual shape. TerminalOutput.tsx owns
 * the 150ms show-delay and 8s stalled timer; this component only renders
 * the state it's given.
 */ function ScrollLoadingPill({ visible, stalled, program, onCancel }) {
    if (!visible) return null;
    const label = stalled ? program ? `Still trying to load ${program}'s history…` : "Still trying to load more…" : program ? `Loading ${program}'s history…` : "Loading more…";
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: scrollLoadingPill,
        role: "status",
        "aria-live": "polite",
        "data-testid": "scroll-loading-pill",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                children: label
            }),
            stalled && // A transient loading-state escape hatch, not a user-initiated
            // feature action — mirrors the untracked reconnect/dismiss controls
            // elsewhere in this file.
            // analytics-exempt
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                className: scrollLoadingPillCancel,
                onClick: onCancel,
                "data-testid": "scroll-loading-pill-cancel",
                children: "Cancel"
            })
        ]
    });
}

;// ./src/components/sessions/ScrollSourceIndicator.tsx
/* __next_internal_client_entry_do_not_use__ ScrollSourceIndicator auto */ 
// +feature: scroll-source-indicator

/**
 * Persistent banner shown while the viewport is displaying an app-forwarded
 * page of a program's own history instead of the live tail (Story 1.4.2,
 * design/ux.md Surface 2). Unlike ScrollLoadingPill this does not auto-dismiss
 * on a timer — TerminalOutput.tsx unmounts it once a non-app-scrollback frame
 * arrives (return to live) or the session disconnects.
 */ function ScrollSourceIndicator({ program, visible }) {
    if (!visible) return null;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: scrollSourceIndicator,
        role: "status",
        "aria-live": "polite",
        "data-testid": "scroll-source-indicator",
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                "aria-hidden": "true",
                children: "\uD83D\uDCDC"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                children: [
                    "Viewing ",
                    program,
                    "'s own history"
                ]
            })
        ]
    });
}

;// ./src/lib/terminal/viewportSettle.ts
/**
 * ViewportSettled signal (Story 2.1.4): fires once both visualViewport height
 * and offsetTop are unchanged for `stableFrames` consecutive frames, or after
 * `maxWaitMs` from the arming event, whichever comes first. Armed by vv
 * `resize` and `scroll`. Pure: scheduler and window are injected; imports
 * nothing from lib/hooks.
 */ const DEFAULT_STABLE_FRAMES = 3;
const DEFAULT_MAX_WAIT_MS = 600;
function createRafScheduler() {
    return {
        request: (cb)=>requestAnimationFrame(()=>cb()),
        cancel: (h)=>cancelAnimationFrame(h),
        now: ()=>performance.now()
    };
}
function windowFallbackSource(win) {
    return {
        get height () {
            return win.innerHeight;
        },
        offsetTop: 0,
        addEventListener: (type, listener)=>{
            if (type === "resize") win.addEventListener("resize", listener);
        },
        removeEventListener: (type, listener)=>{
            if (type === "resize") win.removeEventListener("resize", listener);
        }
    };
}
function resolveFallbackWindow(win) {
    if (win) return win;
    if (false) {}
    throw new Error("createViewportSettle: no visualViewport and no window available; pass options.win");
}
/** Returns a dispose function that unsubscribes and cancels any pending frame. */ function createViewportSettle(vv, scheduler, options) {
    const source = vv ?? windowFallbackSource(resolveFallbackWindow(options.win));
    const { stableFrames, maxWaitMs, onSettled } = options;
    let frameHandle = null;
    let armedAt = 0;
    let lastHeight = NaN;
    let lastOffsetTop = NaN;
    let stableCount = 0;
    let disposed = false;
    const settle = (height, offsetTop)=>{
        frameHandle = null;
        onSettled({
            height,
            offsetTop
        });
    };
    const onFrame = ()=>{
        frameHandle = null;
        if (disposed) return;
        const { height, offsetTop } = source;
        if (height === lastHeight && offsetTop === lastOffsetTop) {
            stableCount++;
        } else {
            lastHeight = height;
            lastOffsetTop = offsetTop;
            stableCount = 1;
        }
        if (stableCount >= stableFrames || scheduler.now() - armedAt >= maxWaitMs) {
            settle(height, offsetTop);
            return;
        }
        frameHandle = scheduler.request(onFrame);
    };
    const onEvent = ()=>{
        if (disposed || frameHandle !== null) return;
        armedAt = scheduler.now();
        lastHeight = NaN;
        lastOffsetTop = NaN;
        stableCount = 0;
        frameHandle = scheduler.request(onFrame);
    };
    source.addEventListener("resize", onEvent);
    source.addEventListener("scroll", onEvent);
    return ()=>{
        disposed = true;
        source.removeEventListener("resize", onEvent);
        source.removeEventListener("scroll", onEvent);
        if (frameHandle !== null) {
            scheduler.cancel(frameHandle);
            frameHandle = null;
        }
    };
}

// EXTERNAL MODULE: ./src/lib/terminal/scrollRouting.ts
var scrollRouting = __webpack_require__(26550);
;// ./src/lib/terminal/scrollPosition.ts
/**
 * Away-from-live detection and the TUI `netPagesUp` estimate (design/ux.md S7).
 * Pure: no DOM, no React. The estimate counts page keys sent up minus down
 * (drag and toolbar); the app owns its real position, so the estimate is only
 * trusted while nothing has happened that could have moved it.
 */ /** Overlays (chip, jump button) are hidden when fewer terminal rows are visible. */ const MIN_ROWS_FOR_OVERLAYS = 5;
/** Beyond this the estimate is too uncertain to send keys from. */ const NET_PAGES_UP_LIMIT = 5;
/** Local buffer only: true when the viewport sits above the live bottom. */ function isAwayFromLive({ viewportY, baseY }) {
    return viewportY < baseY;
}
const LIVE = {
    pages: 0,
    valid: true
};
function createNetPagesUpTracker(limit = NET_PAGES_UP_LIMIT) {
    let state = LIVE;
    const listeners = new Set();
    const set = (next)=>{
        if (next.pages === state.pages && next.valid === state.valid && next.reason === state.reason) return;
        state = next;
        listeners.forEach((l)=>l());
    };
    const invalidate = (reason)=>set({
            pages: 0,
            valid: false,
            reason
        });
    return {
        getState: ()=>state,
        pageUp () {
            if (!state.valid && state.reason === "overflow") return;
            const pages = (state.valid ? state.pages : 0) + 1;
            if (pages > limit) invalidate("overflow");
            else set({
                pages,
                valid: true
            });
        },
        pageDown () {
            if (!state.valid) return;
            set({
                pages: Math.max(0, state.pages - 1),
                valid: true
            });
        },
        invalidate,
        markLive: ()=>set(LIVE),
        subscribe (listener) {
            listeners.add(listener);
            return ()=>{
                listeners.delete(listener);
            };
        }
    };
}

;// ./src/lib/terminal/scrollOverride.ts
/**
 * Persisted scroll settings (design/ux.md S6): the drag-routing override and the
 * Gesture scrolling switch. Per device, not per session. Storage can throw
 * (private mode, quota), so every access falls back to defaults and the
 * in-memory choice keeps applying for the rest of the session.
 */ 
const SCROLL_OVERRIDE_KEY = "terminal-scroll-override";
const GESTURE_SCROLL_KEY = "terminal-gesture-scroll";
const DEFAULT_SCROLL_OVERRIDE = "auto";
const DEFAULT_GESTURE_SCROLL = true;
function defaultStorage() {
    return typeof localStorage === "undefined" ? null : localStorage;
}
function parseOverride(raw) {
    return raw === "local" || raw === "tui" ? raw : DEFAULT_SCROLL_OVERRIDE;
}
function createScrollSettings(options = {}) {
    const getStorage = options.getStorage ?? defaultStorage;
    const onOverrideChange = options.onOverrideChange ?? ((from, to)=>mobileDebug/* mobileDebug */.Uo.overrideChange(from, to));
    const listeners = new Set();
    // Set once the user changes a value, so it wins over a storage that rejected the write.
    let memoryOverride = null;
    let memoryGesture = null;
    const readRaw = (key)=>{
        try {
            return getStorage()?.getItem(key) ?? null;
        } catch  {
            return null;
        }
    };
    const write = (apply)=>{
        try {
            const storage = getStorage();
            if (storage) apply(storage);
        } catch  {
        // keep the in-memory choice
        }
    };
    const notify = ()=>listeners.forEach((l)=>l());
    const getOverride = ()=>memoryOverride ?? parseOverride(readRaw(SCROLL_OVERRIDE_KEY));
    const getGestureScroll = ()=>memoryGesture ?? (readRaw(GESTURE_SCROLL_KEY) === "off" ? false : DEFAULT_GESTURE_SCROLL);
    return {
        getOverride,
        getGestureScroll,
        setOverride (value) {
            const previous = getOverride();
            memoryOverride = value;
            write((s)=>value === "auto" ? s.removeItem(SCROLL_OVERRIDE_KEY) : s.setItem(SCROLL_OVERRIDE_KEY, value));
            if (previous === value) return;
            onOverrideChange(previous, value);
            notify();
        },
        setGestureScroll (on) {
            const previous = getGestureScroll();
            memoryGesture = on;
            write((s)=>on ? s.removeItem(GESTURE_SCROLL_KEY) : s.setItem(GESTURE_SCROLL_KEY, "off"));
            if (previous !== on) notify();
        },
        subscribe (listener) {
            listeners.add(listener);
            return ()=>{
                listeners.delete(listener);
            };
        }
    };
}
/** Shared store for app code. */ const scrollSettings = createScrollSettings();

// EXTERNAL MODULE: ./src/components/ui/LiveRegion.css.ts
var LiveRegion_css = __webpack_require__(40855);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FJumpToLatestButton.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA42Tb4ucMBDG3_sphoPCHjSLurf0zlK471HKEc2o0zV_msSuWvzuxbiyt7oHhy9knmcyM2R-eT1hX1ou0cFbcjzlvfkWw78IIP4SfgDa8IJ8n0H8PQIYI4Ak3prJbI7RfimThBSjHXnSKgOeO920HqdES1XtM_jL7Y6xt-QFh9QO6fA4eQMjJbDLID1OoSBnGt5nQKohhaxssJt03lClGHmULoMClUc7yb9b56nsWaGVR-XfWxU3m5Z9aClJsRrnmZ6eTLdoZxK-vkqXMEn2R4tyUgwXglR1p-xKOcShUc6LU2V1q8T6iAx-oRtt19af-ai2Am0GSiu8xsxyQa1bHzm8hDOlVp45GnDj51f_jPeWcaguXTvmai70OYMYUtPBs-nAVjnfxV_Dtz_Mk7fWTaMbTct9c0WSz7u_opWksXSA3CHTrZ_SvOVqYeSC0ybrPVY_BfecFdoqtD8ecu29liwA9fArIDdLd-n6uI7X5qaI11tYVhXSkHiB4nlGZKHoEn5mSR8zMS9-Ap652pI6hTc4Rq8SBXHYGYslWscsirZAwaSeb3GOH8N4q-d4e90LSze7WsQxGv8D5NvxhR0EAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FJumpToLatestButton_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA42Tb4ucMBDG3_sphoPCHjSLurf0zlK471HKEc2o0zV_msSuWvzuxbiyt7oHhy9knmcyM2R_eT1hX1ou0cFbcjzlvfkWw78IIP4SfgDa8IJ8n0H8PQIYI4Ak3prJbI7RfimThBSjHXnSKgOeO920HqdES1XtM_jL7Y6xt_QFh9QO6fA4eQMjJbDLID1OoSBnGt5nQKohhaxssJt03lClGHmULoMClUc7yb9b56nsWaGVR_XfWxU3m5Z9aClJsRrnmZ6eTLdoZxK_vkqXMEn2R4tyUgwXglR1p_xKOcShUc6LU2V1q8T6iAx_oRtt19af_ai2Am0GSiu8xsxyQa1bHzm8hDOlVp45GnDj51f_jPeWcaguXTvmai70OYMYUtPBs_nAVjnfxV_Dtz_Mk7fWTaMbTct9c0WSz7u_opWksXSA3CHTrZ_SvOVqYeSC0ybrPVY_BfecFdoqtD8ecu29liwA9fArIDdLd_n6uI7X5qaI11tYVhXSkHiB4nlGZKHoEn5mSR8zMS9_Ap652pI6hTc4Rq8SBXHYGYslWscsirZAwaSeb3GOH8N4q_d4e90LSze7WsQxGv8D5NvxhR0EAAA_22_7D = __webpack_require__(34411);
;// ./src/components/sessions/JumpToLatestButton.css.ts


var JumpToLatestButton_css_button = '_15kbyp71';
var newOutputDot = '_15kbyp72';
;// ./src/components/sessions/JumpToLatestButton.tsx
/* __next_internal_client_entry_do_not_use__ JumpToLatestButton auto */ 
// +feature: terminal-jump-to-latest





const NEW_OUTPUT_ANNOUNCE_DELAY_MS = 2000;
const LOCAL_LABEL = "Jump to latest";
const TUI_LABEL = "Page down to latest";
function readSnapshot(terminal) {
    if (!terminal) return {
        rows: 0,
        away: false
    };
    const { viewportY, baseY, length } = terminal.buffer.active;
    return {
        rows: terminal.rows,
        away: length > 0 && isAwayFromLive({
            viewportY,
            baseY
        })
    };
}
function overlaps(a, b) {
    return a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;
}
/**
 * Affordance back to live output (design/ux.md S7). Local buffer: shown while the
 * viewport is above the bottom. TUI: shown only while the page-key estimate is valid
 * and positive. Never focuses the terminal, so the keyboard stays as it was.
 */ function JumpToLatestButton({ terminal, route, netPagesUp, connectionEpoch, sendData, isInputBusy, gestureActive = false, outputTick = 0, getCursorRowRect }) {
    const [snapshot, setSnapshot] = (0,react.useState)(()=>readSnapshot(terminal));
    const estimate = (0,react.useSyncExternalStore)(netPagesUp.subscribe, netPagesUp.getState, netPagesUp.getState);
    const isLocal = route === "xterm-local";
    const refresh = (0,react.useCallback)(()=>{
        setSnapshot((prev)=>{
            const next = readSnapshot(terminal);
            return next.rows === prev.rows && next.away === prev.away ? prev : next;
        });
    }, [
        terminal
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        refresh();
        const subscriptions = [
            terminal?.onScroll?.(refresh),
            terminal?.onWriteParsed?.(refresh)
        ];
        return ()=>subscriptions.forEach((s)=>s?.dispose());
    }, [
        terminal,
        refresh
    ]);
    const visible = snapshot.rows >= MIN_ROWS_FOR_OVERLAYS && (isLocal ? snapshot.away : estimate.valid && estimate.pages > 0);
    // ---- Reconnect: the estimate no longer describes the app's position ----
    const lastEpochRef = (0,react.useRef)(connectionEpoch);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (lastEpochRef.current === connectionEpoch) return;
        lastEpochRef.current = connectionEpoch;
        netPagesUp.invalidate("reconnect");
    }, [
        connectionEpoch,
        netPagesUp
    ]);
    // ---- Placement: evaluated when shown and when a gesture ends, never mid-gesture ----
    const buttonRef = (0,react.useRef)(null);
    const [corner, setCorner] = (0,react.useState)("bottom-right");
    const evaluatePlacement = (0,react.useCallback)(()=>{
        const cursor = getCursorRowRect?.();
        const own = buttonRef.current?.getBoundingClientRect();
        if (!cursor || !own) return;
        const next = overlaps(own, cursor) ? "top-right" : "bottom-right";
        setCorner((c)=>c === next ? c : next);
    }, [
        getCursorRowRect
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useLayoutEffect)(()=>{
        if (visible) evaluatePlacement();
        else setCorner("bottom-right");
    // Deliberately keyed on `visible` only: the corner is kept until the button hides.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        visible
    ]);
    const wasGestureRef = (0,react.useRef)(gestureActive);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (wasGestureRef.current && !gestureActive && visible) evaluatePlacement();
        wasGestureRef.current = gestureActive;
    }, [
        gestureActive,
        visible,
        evaluatePlacement
    ]);
    // ---- Output while scrolled: dot plus one debounced announcement per episode ----
    const [hasNewOutput, setHasNewOutput] = (0,react.useState)(false);
    const [announcement, setAnnouncement] = (0,react.useState)("");
    const announcedRef = (0,react.useRef)(false);
    const announceTimerRef = (0,react.useRef)(null);
    const lastTickRef = (0,react.useRef)(outputTick);
    const clearAnnounceTimer = (0,react.useCallback)(()=>{
        if (announceTimerRef.current) clearTimeout(announceTimerRef.current);
        announceTimerRef.current = null;
    }, []);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (lastTickRef.current === outputTick) return;
        lastTickRef.current = outputTick;
        if (!visible) return;
        setHasNewOutput(true);
        if (announcedRef.current || announceTimerRef.current) return;
        announceTimerRef.current = setTimeout(()=>{
            announceTimerRef.current = null;
            announcedRef.current = true;
            setAnnouncement("New output below");
        }, NEW_OUTPUT_ANNOUNCE_DELAY_MS);
    }, [
        outputTick,
        visible
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (visible) return;
        clearAnnounceTimer();
        announcedRef.current = false;
        setHasNewOutput(false);
        setAnnouncement("");
    }, [
        visible,
        clearAnnounceTimer
    ]);
    // ---- Tap ----
    const pageKeyTimersRef = (0,react.useRef)(new Set());
    const cancelPageKeyTimers = (0,react.useCallback)(()=>{
        pageKeyTimersRef.current.forEach(clearTimeout);
        pageKeyTimersRef.current.clear();
    }, []);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>()=>{
            cancelPageKeyTimers();
            clearAnnounceTimer();
        }, [
        cancelPageKeyTimers,
        clearAnnounceTimer
    ]);
    // Queued extra PgDn keys target a position that no longer holds once the estimate is invalidated
    // (keystroke, resize, mode change, reconnect), the epoch bumps, or the route flips.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!estimate.valid) cancelPageKeyTimers();
    }, [
        estimate.valid,
        cancelPageKeyTimers
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>cancelPageKeyTimers, [
        route,
        connectionEpoch,
        cancelPageKeyTimers
    ]);
    const handleClick = ()=>{
        if (isLocal) {
            terminal?.scrollToBottom();
            return;
        }
        if (isInputBusy?.()) return;
        const count = Math.min(estimate.pages, NET_PAGES_UP_LIMIT);
        netPagesUp.markLive();
        for(let i = 0; i < count; i++){
            const send = ()=>{
                if (!isInputBusy?.()) sendData((0,scrollRouting/* encodePageKeys */.qC)("newer"));
            };
            if (i === 0) {
                send();
                continue;
            }
            const timers = pageKeyTimersRef.current;
            const id = setTimeout(()=>{
                timers.delete(id);
                send();
            }, scrollRouting/* PAGE_RATE_LIMIT_MS */.Mu * i);
            timers.add(id);
        }
    };
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                role: "status",
                "aria-live": "polite",
                className: LiveRegion_css/* srOnly */.s,
                children: announcement
            }),
            visible && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                ref: buttonRef,
                type: "button",
                className: JumpToLatestButton_css_button,
                "data-corner": corner,
                onMouseDown: (e)=>e.preventDefault(),
                onClick: handleClick,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        "aria-hidden": "true",
                        children: "↓"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                        children: isLocal ? LOCAL_LABEL : TUI_LABEL
                    }),
                    hasNewOutput && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: newOutputDot,
                                "data-testid": "jump-new-output-dot",
                                "aria-hidden": "true"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: LiveRegion_css/* srOnly */.s,
                                children: "new output"
                            })
                        ]
                    })
                ]
            })
        ]
    });
}

;// ./src/components/sessions/JumpToLatestMount.tsx
/* __next_internal_client_entry_do_not_use__ JumpToLatestMount auto */ 
// +feature: terminal-jump-to-latest


/**
 * Owns the per-frame output tick so streaming output re-renders only this subtree,
 * not the whole TerminalOutput.
 */ function JumpToLatestMount({ terminal, getContainer, ...rest }) {
    const [outputTick, setOutputTick] = (0,react.useState)(0);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!terminal?.onWriteParsed) return;
        let frame = 0;
        const subscription = terminal.onWriteParsed(()=>{
            if (frame) return;
            frame = requestAnimationFrame(()=>{
                frame = 0;
                setOutputTick((t)=>t + 1);
            });
        });
        return ()=>{
            subscription.dispose();
            if (frame) cancelAnimationFrame(frame);
        };
    }, [
        terminal
    ]);
    const getCursorRowRect = (0,react.useCallback)(()=>{
        const container = getContainer();
        const cursorY = terminal?.buffer.active?.cursorY;
        if (!container || !terminal || cursorY === undefined || terminal.rows <= 0) return null;
        const rect = container.getBoundingClientRect();
        const cellHeight = rect.height / terminal.rows;
        const top = rect.top + cursorY * cellHeight;
        return {
            left: rect.left,
            right: rect.right,
            top,
            bottom: top + cellHeight
        };
    }, [
        terminal,
        getContainer
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)(JumpToLatestButton, {
        terminal: terminal,
        outputTick: outputTick,
        getCursorRowRect: getCursorRowRect,
        ...rest
    });
}

// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@vanilla-extract+webpack-plugin@2.3.26+db3e8d9ee620131b/node_modules/@vanilla-extract/webpack-plugin/vanilla.virtual.css?%7B%22fileName%22%3A%22src%2Fcomponents%2Fsessions%2FScrollingPanel.css.ts.vanilla.css%22%2C%22source%22%3A%22%23H4sIAAAAAAAAA8VW246bMBB9z1dY-9Q8eBVIttsQ9UuqamXwANM1NrKHJKTqv1cYaEIuu3STtMpDsGeYOTPneMzjS1gLMsGM_ZwwJtGVStQRSxVsVxPm_7lECwmh0RFLjKoK3VgyUUZsLewnzl-CJexCu5vPpo2lFFKizi5YY5G8ZtZUWh47PHt7YpSxx6buVWMl2IgF5ZY5o1AeecGBF7dCYuVOQLRJUqOJO9zBiT2eria_Jo99V75JQYKbNVgl6q8PZCt4-O5bVRqHbU9E7IyqCJrAClKK2Kx5tJjl_fOOo5awjVj41CybeKkyG15HTFRkDlMGPnpfqjbaxy2EzVB30f402K-u4yzcDQoOfXbfng20BRw1KJueQmjR8dgQmeKdDPPzQhMKM82RoHDtJnckLL2ttAI1zzuYi0W57fc2KCnfb10QpAfGWFJZ1yiuNKgJ7CHWRdsNjya3qF-7erv4YRe_h9Cve7KCWblls-Y3CBqlJqkcX6PDWIFPYSpSqCFi4QVpFx5q58ZNmjog730Y-ekGZzisB2x9HqWHvf-z9z97hoOB45c7y-DDnC89sMF5G1GOaN-6To_19P2RSVZoVwoLmq6clvmNpuWb3RR3E3s8FBBq73xWRwm0sBj7UTnCtOaJ0QSaDk3_gLsbX3djCFx-jEDGNjkScFeKBJpbaGNFuTozCw_4uB_T7SWcY5arhh-Qg4u4a0DH07ylafzESkYMor1K_vqrpxs4_1kGo756fgPI4WKeCwoAAA%22%7D
var vanilla_virtual_7B_22fileName_22_3A_22src_2Fcomponents_2Fsessions_2FScrollingPanel_css_ts_vanilla_css_22_2C_22source_22_3A_22_23H4sIAAAAAAAAA8VW246bMBB9z1dY_9Q8eBVIttsQ9UuqamXwANM1NrKHJKTqv1cYaEIuu3STtMpDsGeYOTPneMzjS1gLMsGM_ZwwJtGVStQRSxVsVxPm_7lECwmh0RFLjKoK3VgyUUZsLewnzl_CJexCu5vPpo2lFFKizi5YY5G8ZtZUWh47PHt7YpSxx6buVWMl2IgF5ZY5o1AeecGBF7dCYuVOQLRJUqOJO9zBiT2eria_Jo99V75JQYKbNVgl6q8PZCt4_O5bVRqHbU9E7IyqCJrAClKK2Kx5tJjl_fOOo5awjVj41CybeKkyG15HTFRkDlMGPnpfqjbaxy2EzVB30f402K_u4yzcDQoOfXbfng20BRw1KJueQmjR8dgQmeKdDPPzQhMKM82RoHDtJnckLL2ttAI1zzuYi0W57fc2KCnfb10QpAfGWFJZ1yiuNKgJ7CHWRdsNjya3qF_7erv4YRe_h9Cve7KCWblls_Y3CBqlJqkcX6PDWIFPYSpSqCFi4QVpFx5q58ZNmjog730Y_ekGZzisB2x9HqWHvf_z9z97hoOB45c7y_DDnC89sMF5G1GOaN_6To_19P2RSVZoVwoLmq6clvmNpuWb3RR3E3s8FBBq73xWRwm0sBj7UTnCtOaJ0QSaDk3_gLsbX3djCFx_jEDGNjkScFeKBJpbaGNFuTozCw_4uB_T7SWcY5arhh_Qg4u4a0DH07ylafzESkYMor1K_vqrpxs4_1kGo756fgPI4WKeCwoAAA_22_7D = __webpack_require__(21817);
;// ./src/components/sessions/ScrollingPanel.css.ts


var ScrollingPanel_css_button = '_2yato1a';
var chip = '_2yato1b';
var description = '_2yato17';
var fieldset = '_2yato11';
var hint = '_2yato1c';
var legend = '_2yato12';
var note = '_2yato19';
var ScrollingPanel_css_option = '_2yato13';
var optionInput = '_2yato14';
var optionLabel = '_2yato16';
var optionText = '_2yato15';
var ScrollingPanel_css_panel = '_2yato10';
var switchRow = '_2yato18';
;// ./src/components/sessions/ScrollHint.tsx
/* __next_internal_client_entry_do_not_use__ SCROLL_HINT_SEEN_KEY,HINT_TEXT_LOCAL,HINT_TEXT_TUI,useScrollHint,ScrollHint auto */ 
// +feature: terminal-scroll-settings


const SCROLL_HINT_SEEN_KEY = "terminal-scroll-hint-seen";
const HINT_TEXT_LOCAL = "Dragging scrolls the terminal's history. Tap the chip to change.";
const HINT_TEXT_TUI = "Dragging sends Page Up/Down to the app. Tap the chip to change.";
function ScrollHint_defaultStorage() {
    return typeof localStorage === "undefined" ? null : localStorage;
}
/**
 * First-use hint state. Shown on the first scroll start while the seen flag is unset; stays
 * (no timeout) until dismissed or the picker/panel opens. The flag is written then, not on display.
 */ function useScrollHint(options = {}) {
    const getStorage = options.storage ?? ScrollHint_defaultStorage;
    const memorySeen = (0,react.useRef)(false);
    const [visible, setVisible] = (0,react.useState)(false);
    const [route, setRoute] = (0,react.useState)("xterm-local");
    const isSeen = (0,react.useCallback)(()=>{
        if (memorySeen.current) return true;
        try {
            return getStorage()?.getItem(SCROLL_HINT_SEEN_KEY) === "1";
        } catch  {
            return false;
        }
    }, [
        getStorage
    ]);
    const markSeen = (0,react.useCallback)(()=>{
        memorySeen.current = true;
        try {
            getStorage()?.setItem(SCROLL_HINT_SEEN_KEY, "1");
        } catch  {
        // in-memory flag still applies for this page load
        }
    }, [
        getStorage
    ]);
    const notifyScrollStart = (0,react.useCallback)((next)=>{
        if (isSeen()) return;
        setRoute(next);
        setVisible(true);
    }, [
        isSeen
    ]);
    const dismiss = (0,react.useCallback)(()=>{
        markSeen();
        setVisible(false);
    }, [
        markSeen
    ]);
    return {
        visible,
        route,
        notifyScrollStart,
        dismiss,
        markSeenOnPanelOpen: dismiss
    };
}
/** The status region stays mounted so the hint's appearance is announced once. */ function ScrollHint({ visible, route, onDismiss }) {
    return /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
        role: "status",
        "aria-live": "polite",
        children: visible && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
            className: hint,
            "data-testid": "scroll-hint",
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                    children: route === "xterm-local" ? HINT_TEXT_LOCAL : HINT_TEXT_TUI
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                    type: "button",
                    className: ScrollingPanel_css_button,
                    onClick: onDismiss,
                    children: "Got it"
                })
            ]
        })
    });
}

;// ./src/components/sessions/ScrollModeChip.tsx
/* __next_internal_client_entry_do_not_use__ MISROUTE_HIGHLIGHT_MS,MISROUTE_ANNOUNCE_INTERVAL_MS,MISROUTE_ANNOUNCEMENT,routeLabel,useTouchCapable,useMisrouteCue,ScrollModeChip auto */ 
// +feature: terminal-scroll-settings




const MISROUTE_HIGHLIGHT_MS = 5000;
const MISROUTE_ANNOUNCE_INTERVAL_MS = 30000;
const MISROUTE_ANNOUNCEMENT = "The drag did not scroll. Scroll mode is Terminal history. Tap the chip to change.";
function routeLabel(target) {
    return target === "xterm-local" ? "Terminal history" : "Page keys";
}
/**
 * True when the device has a coarse pointer or a touchstart has been seen on the surface.
 * `getSurface` is read on mount; without a surface the document is watched instead.
 */ function useTouchCapable(getSurface) {
    const [touchSeen, setTouchSeen] = (0,react.useState)(false);
    const coarse =  false ? 0 : false;
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const target = getSurface?.() ?? document;
        const onTouch = ()=>setTouchSeen(true);
        target.addEventListener("touchstart", onTouch, {
            passive: true,
            once: true
        });
        return ()=>target.removeEventListener("touchstart", onTouch);
    }, [
        getSurface
    ]);
    return coarse || touchSeen;
}
/** Misroute cue (ux.md S6): a local drag that moved nothing highlights the chip and announces at most once per interval. */ function useMisrouteCue(options = {}) {
    const { now = Date.now, highlightMs = MISROUTE_HIGHLIGHT_MS, announceIntervalMs = MISROUTE_ANNOUNCE_INTERVAL_MS } = options;
    const [highlighted, setHighlighted] = (0,react.useState)(false);
    const [announcement, setAnnouncement] = (0,react.useState)("");
    const timer = (0,react.useRef)(null);
    const lastAnnounceAt = (0,react.useRef)(null);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>()=>{
            if (timer.current) clearTimeout(timer.current);
        }, []);
    const report = (0,react.useCallback)(({ route, postSlopLines, viewportYChanged })=>{
        if (route !== "xterm-local" || postSlopLines < 1 || viewportYChanged) return;
        setHighlighted(true);
        if (timer.current) clearTimeout(timer.current);
        timer.current = setTimeout(()=>{
            setHighlighted(false);
            setAnnouncement("");
            timer.current = null;
        }, highlightMs);
        const t = now();
        if (lastAnnounceAt.current === null || t - lastAnnounceAt.current >= announceIntervalMs) {
            lastAnnounceAt.current = t;
            setAnnouncement(MISROUTE_ANNOUNCEMENT);
        }
    }, [
        now,
        highlightMs,
        announceIntervalMs
    ]);
    return {
        highlighted,
        announcement,
        report
    };
}
function ScrollModeChip(props) {
    const touchCapable = useTouchCapable(props.getTouchSurface);
    if (!touchCapable || props.visibleRows < MIN_ROWS_FOR_OVERLAYS) return null;
    const label = routeLabel(props.effectiveTarget);
    const gestures = props.gestureScrollEnabled ? "on" : "off";
    const text = `${props.highlighted ? "! " : ""}${label}${props.gestureScrollEnabled ? "" : ", gestures off"}`;
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                ref: props.buttonRef,
                className: chip,
                "data-highlighted": props.highlighted ? "true" : "false",
                "aria-label": `Scroll mode: ${label}. Touch gestures ${gestures}. Opens scroll settings`,
                onClick: props.onClick,
                children: text
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                role: "status",
                "aria-live": "polite",
                className: LiveRegion_css/* srOnly */.s,
                "data-testid": "scroll-chip-announcer",
                children: props.suppressAnnouncements ? "" : props.announcement ?? ""
            })
        ]
    });
}

;// ./src/components/sessions/ScrollingPanel.tsx
/* __next_internal_client_entry_do_not_use__ ScrollModeChip,useTouchCapable,useMisrouteCue,routeLabel,shouldRenderPanelAsOverlay,SCROLL_OPTIONS,GESTURE_DESCRIPTION,WIDE_OUTPUT_NOTE,TMUX_NOTE,ScrollingPanel auto */ 
// +feature: terminal-scroll-settings





/** True when opening the panel inline would leave too few terminal rows, so it must overlay instead. */ function shouldRenderPanelAsOverlay(rowsLeftIfInline) {
    return rowsLeftIfInline < MIN_ROWS_FOR_OVERLAYS;
}
const SCROLL_OPTIONS = [
    {
        value: "auto",
        label: "Auto (recommended)",
        announce: "Auto",
        description: "Picks for you based on what is running."
    },
    {
        value: "local",
        label: "Terminal history",
        announce: "Terminal history",
        description: "Dragging moves the terminal's own scrollback, following your finger. Use in a plain shell."
    },
    {
        value: "tui",
        label: "Page keys",
        announce: "Page keys",
        description: "Dragging sends Page Up/Down to the app, one page per half-screen. Use for Claude Code or tmux history."
    }
];
const GESTURE_DESCRIPTION = "Turn off if you use a screen reader (TalkBack). Off disables all touch gestures on the terminal: drag scrolling, tap to focus, double-tap and long-press selection. Use the PgUp/PgDn keys to scroll.";
const WIDE_OUTPUT_NOTE = "Long lines wrap; wide output cannot be panned sideways.";
const TMUX_NOTE = "In tmux, Page keys scroll tmux history. If typing seems ignored, tap PgDn until you reach the bottom. Esc is not offered because it interrupts a running Claude Code turn.";
function ScrollingPanel(props) {
    const { variant, override, effectiveTarget, gestureScrollEnabled, openerRef, renderAsOverlay, maxHeight } = props;
    const uid = (0,react.useId)();
    const [announcement, setAnnouncement] = (0,react.useState)("");
    const returnFocus = (0,react.useCallback)(()=>openerRef?.current?.focus(), [
        openerRef
    ]);
    const close = ()=>{
        props.onClose();
        returnFocus();
    };
    const select = (opt)=>{
        props.onOverrideChange(opt.value);
        setAnnouncement(`Scroll mode: ${opt.announce}`);
        if (variant === "picker") close();
    };
    const nowText = effectiveTarget === "xterm-local" ? "Terminal history" : "Page keys";
    const isFull = variant === "full";
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: ScrollingPanel_css_panel,
        "data-testid": `scrolling-${variant}`,
        "data-overlay": renderAsOverlay,
        style: renderAsOverlay && maxHeight !== undefined ? {
            maxHeight
        } : undefined,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("fieldset", {
                className: fieldset,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("legend", {
                        className: legend,
                        children: "How dragging scrolls"
                    }),
                    SCROLL_OPTIONS.map((opt)=>{
                        const id = `${uid}-${opt.value}`;
                        const descId = `${id}-desc`;
                        return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                            className: ScrollingPanel_css_option,
                            htmlFor: id,
                            children: [
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                    id: id,
                                    className: optionInput,
                                    type: "radio",
                                    name: `${uid}-scroll-mode`,
                                    value: opt.value,
                                    checked: override === opt.value,
                                    onChange: ()=>select(opt),
                                    "aria-describedby": descId
                                }),
                                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                                    className: optionText,
                                    children: [
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                            className: optionLabel,
                                            children: opt.label
                                        }),
                                        opt.value === "auto" && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                                            children: [
                                                "now: ",
                                                nowText
                                            ]
                                        }),
                                        /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                            id: descId,
                                            className: description,
                                            children: opt.description
                                        })
                                    ]
                                })
                            ]
                        }, opt.value);
                    })
                ]
            }),
            isFull && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("label", {
                        className: switchRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                type: "checkbox",
                                role: "switch",
                                checked: gestureScrollEnabled,
                                onChange: (e)=>{
                                    props.onGestureScrollChange(e.target.checked);
                                    setAnnouncement(`Touch gestures: ${e.target.checked ? "on" : "off"}`);
                                },
                                "aria-describedby": `${uid}-gesture-desc`,
                                className: optionInput
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                                className: optionText,
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                        className: optionLabel,
                                        children: "Touch gestures (scroll, select)"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                        id: `${uid}-gesture-desc`,
                                        className: description,
                                        children: GESTURE_DESCRIPTION
                                    })
                                ]
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: note,
                        children: WIDE_OUTPUT_NOTE
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("p", {
                        className: note,
                        children: TMUX_NOTE
                    })
                ]
            }),
            !isFull && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                className: ScrollingPanel_css_button,
                onClick: props.onOpenFull,
                children: "More scrolling settings"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                type: "button",
                className: ScrollingPanel_css_button,
                onClick: close,
                children: "Close"
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                role: "status",
                "aria-live": "polite",
                className: LiveRegion_css/* srOnly */.s,
                "data-testid": "scrolling-announcer",
                children: announcement
            })
        ]
    });
}

;// ./src/lib/hooks/useEffectiveScrollMode.ts



/**
 * Single source for the route the toolbar PgUp/PgDn, chip and jump button follow.
 * The drag hook does not use it: it re-reads the terminal every frame.
 * Turning gestures off never changes the route, only `gesturesOn`.
 */ function useEffectiveScrollMode(scrollMode, override, gestureOn, tuiPolicy) {
    const { bufferType, mouseTrackingMode } = scrollMode;
    return (0,react.useMemo)(()=>({
            target: (0,scrollRouting/* decideScrollTarget */.H$)({
                bufferType,
                mouseTrackingMode
            }, undefined, tuiPolicy, override),
            source: override === "auto" ? "auto" : "override",
            gesturesOn: gestureOn
        }), [
        bufferType,
        mouseTrackingMode,
        override,
        gestureOn,
        tuiPolicy
    ]);
}
/** Live view of the persisted override and Gesture scrolling switch. */ function useScrollSettings(store = scrollSettings) {
    const override = (0,react.useSyncExternalStore)(store.subscribe, store.getOverride, ()=>DEFAULT_SCROLL_OVERRIDE);
    const gestureScrollEnabled = (0,react.useSyncExternalStore)(store.subscribe, store.getGestureScroll, ()=>DEFAULT_GESTURE_SCROLL);
    return (0,react.useMemo)(()=>({
            override,
            gestureScrollEnabled
        }), [
        override,
        gestureScrollEnabled
    ]);
}

// EXTERNAL MODULE: ./src/lib/contexts/ApprovalsContext.tsx + 2 modules
var ApprovalsContext = __webpack_require__(36159);
// EXTERNAL MODULE: ./src/components/providers/ViewportProvider.tsx
var ViewportProvider = __webpack_require__(3697);
// EXTERNAL MODULE: ./src/lib/hooks/useInputModeOverride.ts
var useInputModeOverride = __webpack_require__(67535);
// EXTERNAL MODULE: ../../../../../shared-node-modules/69d94c9ed476ffe6946843673b2324c7111a65b5fe181ca8c0284db91d4212f1/node_modules/.bun/@connectrpc+connect-web@2.1.1+0149f91553ca585e/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js + 8 modules
var connect_transport = __webpack_require__(14917);
;// ./src/components/sessions/TerminalOutput.tsx
/* __next_internal_client_entry_do_not_use__ TerminalOutput auto */ 
// +feature: terminal-pre-sizing terminal-dimension-cache terminal-image-upload













































// Minimum dimensions considered "real" — anything smaller is a transient value
// from xterm.js before the CSS container has finished laying out. The first
// resize event often fires at e.g. 10x6 before layout is complete; caching or
// connecting at those dimensions produces a garbled terminal on the next view.
// Read-only view: a keypress announces "Read-only session" at most once per this window (design/ux.md Surface 12).
const READ_ONLY_HINT_INTERVAL_MS = 10000;
// Story 2.3 — coalescing window for InputDropBadge drop episodes (design/ux.md §2.2).
const DROP_EPISODE_COALESCE_WINDOW_MS = 400;
// Matches XtermTerminal's assumed first scroll mode, so only real changes are reported.
const INITIAL_SCROLL_MODE = {
    bufferType: "normal",
    mouseTrackingMode: "none"
};
const SCROLL_PICKER_PANEL_ROWS = 10;
const SCROLL_FULL_PANEL_ROWS = 20;
/**
 * Story 1.4.3 — the exact toast copy for a BLOCKED outcome, keyed on
 * ScrollBlockedReason. Three distinct strings, not one generic "blocked"
 * catch-all (adversarial-review BLOCKER: a solo user on
 * PathLegacyPerConnection, which is unconditionally BLOCKED regardless of
 * actual viewer count, must never see the "another viewer is connected"
 * claim). Copy matches design/ux.md Surface 3 / plan.md Story 1.4.3 verbatim.
 */ function blockedToastCopy(reason, program) {
    const label = program || "this session";
    switch(reason){
        case events_pb/* ScrollBlockedReason */.ec.UNSUPPORTED_STREAMING_PATH:
            return "Scroll-forwarding isn't available for this session yet";
        case events_pb/* ScrollBlockedReason */.ec.LEASE_CONTENTION:
            return "Still loading — try again in a moment";
        case events_pb/* ScrollBlockedReason */.ec.MULTIPLE_VIEWERS:
        case events_pb/* ScrollBlockedReason */.ec.SCROLL_BLOCKED_REASON_UNSPECIFIED:
        default:
            return `Can't browse ${label}'s history right now — another viewer is connected. Close other tabs or sessions viewing this session to enable it.`;
    }
}
function TerminalOutput({ sessionId, baseUrl, isExternal = false, tmuxSessionName, isVisible, shellId, onShellStatusChange, scheduleResync, readOnly = false }) {
    const { track } = (0,AnalyticsContext/* useAnalytics */.st)();
    const { announce } = (0,useAnnounce/* useAnnounce */.XE)();
    const { clearForSession, refresh: refreshApprovals, pendingCount } = (0,ApprovalsContext/* useApprovalsContext */.B)();
    const { leftHanded, toggleHandedness } = (0,useHandedness/* useHandedness */.V)();
    const terminalContainerRef = (0,react.useRef)(null);
    const toolbarRef = (0,react.useRef)(null);
    const overflowRowRef = (0,react.useRef)(null);
    // Only the visible terminal anchors the phone deck; a background pane must not move it.
    (0,usePublishStackTopOffset/* usePublishDeckTopOffset */.K)(toolbarRef, isVisible !== false, overflowRowRef);
    // Story 3 (Task 3.4) — anchor <div> the pooled terminal docks into. A
    // plain, otherwise-childless node (see TerminalPool.tsx's module doc
    // comment for why it must never receive JSX children of its own): the
    // pool imperatively appends/removes its host node here, which would race
    // with React's own reconciliation if this div had other React-managed
    // children.
    const dockRef = (0,react.useRef)(null);
    // Computed early (needs only props) so it's available to usePooledTerminal
    // below, which must itself run early -- see that hook's call site comment.
    const effectiveSessionId = isExternal && tmuxSessionName ? tmuxSessionName : sessionId;
    // Story 3 (Task 3.4/3.5) — replaces a locally-owned
    // `useRef<XtermTerminalHandle | null>(null)` + directly-rendered
    // `<XtermTerminal>`. Returns a stable ref into a pool-owned, possibly
    // already-warm Terminal instance that survives this component's own
    // remounts (e.g. a pane's assigned session changing -- see
    // TerminalPool.tsx's module doc comment for why that matters here).
    // isVisible defaults to visible (`!== false`) to match this file's other
    // isVisible-optional call sites (e.g. the loading overlay below).
    const { xtermRef, warmRef } = (0,TerminalPool/* usePooledTerminal */.L)(effectiveSessionId, dockRef, isVisible !== false);
    // Track actual rendered pane dimensions via ResizeObserver. Fires after CSS layout
    // has constrained the container to its real pane width (not the full browser window).
    // Used to guard connect() and pre-sizing so we never send wrong dimensions in the handshake.
    const containerSize = useSplitContainerSize(terminalContainerRef);
    const [connectionAttempts, setConnectionAttempts] = (0,react.useState)(0);
    const [showReconnectButton, setShowReconnectButton] = (0,react.useState)(false);
    const [isWaitingForStableSize, setIsWaitingForStableSize] = (0,react.useState)(true);
    // Story 3 (Task 3.4) — a warm pool entry (already showing content from a
    // prior mount) skips the loading overlay entirely; only a genuinely cold
    // entry starts in the loading state. See PoolEntry.warmRef's doc comment.
    const [isLoadingInitialContent, setIsLoadingInitialContent] = (0,react.useState)(()=>!warmRef.current);
    const reconnectTimeoutRef = (0,react.useRef)(null);
    const previousConnectionStateRef = (0,react.useRef)(false);
    const lastResizeRef = (0,react.useRef)(null);
    const refreshCountRef = (0,react.useRef)(0);
    const isMountedRef = (0,react.useRef)(true);
    // One-shot: set when a viewport settle triggers refit(); the single resize that fit produces bypasses the
    // server bounce hold (Story 2.1.5) and consumes it. Any later resize, e.g. an A->B->A oscillation, is held.
    const settleBypassPendingRef = (0,react.useRef)(false);
    const clearedAtRef = (0,react.useRef)(null);
    const sizeStabilityTimeoutRef = (0,react.useRef)(null);
    const hasInitiatedConnectionRef = (0,react.useRef)(false);
    const hasCachedDimensionsRef = (0,react.useRef)(false);
    // Set to true when we switch sessions while connected; triggers connect() once disconnect completes
    const pendingConnectAfterDisconnectRef = (0,react.useRef)(false);
    // Story 3.2.1 — reconnecting banner state
    const hasEverConnectedRef = (0,react.useRef)(false);
    const bannerTimerRef = (0,react.useRef)(null);
    const [showReconnectBanner, setShowReconnectBanner] = (0,react.useState)(false);
    // Debounce timer for the approval re-fetch triggered by any keystroke (ADR-4)
    const refreshTimerRef = (0,react.useRef)(null);
    // TerminalStreamManager ref -- lazily initialized when terminal is available
    const streamManagerRef = (0,react.useRef)(null);
    // Ref to hold sendFlowControl function (allows use in callbacks defined before useTerminalStream)
    const sendFlowControlRef = (0,react.useRef)(null);
    // Task 4.2.2 — Queue output during RESIZING state (Pitfall #2 / Race 3).
    // A snapshot arriving while the client is mid-resize writes bytes at the old column width.
    // Queuing output during resize prevents stale bytes from being written before the post-resize snapshot.
    const pendingOutputDuringResizeRef = (0,react.useRef)([]);
    // Terminal loading metrics
    const metricsRef = (0,react.useRef)({
        mountTime: performance.now(),
        firstResizeTime: null,
        sizeStableTime: null,
        connectionInitTime: null,
        connectedTime: null,
        firstOutputTime: null,
        resizeCount: 0
    });
    // xterm.js preload moved to TerminalPoolProvider (Story 3) -- it's now the
    // sole mount point for <XtermTerminal>, so the preload only needs to run
    // once app-wide rather than per TerminalOutput mount.
    const logTerminalMetrics = (0,react.useCallback)(()=>{
        const m = metricsRef.current;
        if (!m.firstOutputTime) return;
        const metrics = {
            totalLoadTime: m.firstOutputTime - m.mountTime,
            timeToFirstResize: m.firstResizeTime ? m.firstResizeTime - m.mountTime : null,
            timeToSizeStable: m.sizeStableTime ? m.sizeStableTime - m.mountTime : null,
            sizeStabilizationDuration: m.sizeStableTime && m.firstResizeTime ? m.sizeStableTime - m.firstResizeTime : null,
            timeToConnectionInit: m.connectionInitTime ? m.connectionInitTime - m.mountTime : null,
            timeToConnected: m.connectedTime ? m.connectedTime - m.mountTime : null,
            connectionDuration: m.connectedTime && m.connectionInitTime ? m.connectedTime - m.connectionInitTime : null,
            timeToFirstOutput: m.firstOutputTime - m.mountTime,
            resizeEventsBeforeStable: m.resizeCount,
            sessionId: isExternal ? tmuxSessionName : sessionId,
            isExternal
        };
        console.log(`[TerminalMetrics] Terminal loaded in ${metrics.totalLoadTime.toFixed(0)}ms`, {
            breakdown: {
                sizeStabilization: `${metrics.sizeStabilizationDuration?.toFixed(0) || 'N/A'}ms (${metrics.resizeEventsBeforeStable} resizes)`,
                wsConnection: `${metrics.connectionDuration?.toFixed(0) || 'N/A'}ms`,
                totalLoad: `${metrics.totalLoadTime.toFixed(0)}ms`
            },
            detailed: metrics
        });
        if (false) {}
    }, [
        sessionId,
        tmuxSessionName,
        isExternal
    ]);
    // Debug mode state
    const [debugMode, setDebugMode] = (0,react.useState)(()=>{
        if (false) {}
        return false;
    });
    // Remote log streaming state — persisted in localStorage
    const [logStreamEnabled, setLogStreamEnabled] = (0,react.useState)(()=>{
        if (false) {}
        return false;
    });
    // Sticky modifier keys — CTRL, ALT, SHIFT arm on first tap, fire+clear on the next key.
    // Mutually exclusive: arming one disarms the others.
    const [ctrlActive, setCtrlActive] = (0,react.useState)(false);
    const [altActive, setAltActive] = (0,react.useState)(false);
    const [shiftActive, setShiftActive] = (0,react.useState)(false);
    // Transient paste error shown briefly when clipboard access is denied.
    const [pasteError, setPasteError] = (0,react.useState)(null);
    const pasteErrorTimerRef = (0,react.useRef)(null);
    // Multi-file upload state for the three-button toolbar picker.
    const cameraInputRef = (0,react.useRef)(null);
    const galleryInputRef = (0,react.useRef)(null);
    const filesInputRef = (0,react.useRef)(null);
    const [uploadingCount, setUploadingCount] = (0,react.useState)(0);
    const [uploadError, setUploadError] = (0,react.useState)(null);
    const [uploadSuccess, setUploadSuccess] = (0,react.useState)(null);
    const uploadErrorTimerRef = (0,react.useRef)(null);
    const uploadSuccessTimerRef = (0,react.useRef)(null);
    // Mobile keyboard visibility — persisted in localStorage, scoped per session
    const keyboardStorageKey = `stapler-squad-mobile-keyboard-visible-${sessionId}`;
    const [isKeyboardVisible, setIsKeyboardVisible] = (0,react.useState)(()=>{
        if (true) return true;
        try {
            const stored = localStorage.getItem(keyboardStorageKey);
            return stored === null ? true : stored === 'true';
        } catch  {
            return true;
        }
    });
    const toggleMobileKeyboard = (0,react.useCallback)(()=>{
        setIsKeyboardVisible((prev)=>{
            const next = !prev;
            try {
                localStorage.setItem(keyboardStorageKey, String(next));
            } catch  {
            // localStorage full or disabled — continue without persistence
            }
            return next;
        });
    }, [
        keyboardStorageKey
    ]);
    // Mobile detection — use shared ViewportProvider hook for consistency
    const { isMobile, hasFinePointer } = (0,ViewportProvider/* useViewport */.k)();
    // User-overridable detection for a real mouse+keyboard attached to a phone/tablet —
    // see Settings > Appearance > Terminal Input Mode.
    const { inputModeOverride } = (0,useInputModeOverride/* useInputModeOverride */.t)();
    const compactToolbar = inputModeOverride === 'desktop' || inputModeOverride === 'auto' && isMobile && hasFinePointer;
    // Toolbar collapsed/expanded state — persisted in localStorage; collapsed by default
    const [toolbarExpanded, setToolbarExpanded] = (0,react.useState)(()=>{
        if (true) return false;
        try {
            const s = localStorage.getItem('stapler-squad-toolbar-expanded');
            return s === null ? false : s === 'true';
        } catch  {
            return false;
        }
    });
    const [mobileOverflowOpen, setMobileOverflowOpen] = (0,react.useState)(false);
    // The first time a mouse+physical keyboard is detected on this mobile session,
    // collapse the toolbar and hide the on-screen keyboard row by default so they
    // don't eat screen space — but only if the user hasn't already made an explicit
    // choice for either. Runs once per session mount; manual toggles afterward are
    // fully respected.
    const appliedCompactDefaultsRef = (0,react.useRef)(false);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!compactToolbar || appliedCompactDefaultsRef.current) return;
        appliedCompactDefaultsRef.current = true;
        setToolbarExpanded(false);
        try {
            if (localStorage.getItem(keyboardStorageKey) === null) {
                setIsKeyboardVisible(false);
            }
        } catch  {
        // localStorage unavailable — leave the on-screen keyboard row visible
        }
    }, [
        compactToolbar,
        keyboardStorageKey
    ]);
    // Dev tools panel — persisted in localStorage; collapsed by default
    const [devGroupOpen, setDevGroupOpen] = (0,react.useState)(()=>{
        if (true) return false;
        try {
            return localStorage.getItem('stapler-squad-dev-toolbar') === 'true';
        } catch  {
            return false;
        }
    });
    const devToggleRef = (0,react.useRef)(null);
    const handleDevGroupToggle = ()=>{
        const next = !devGroupOpen;
        track({
            name: "toolbar_button_click",
            category: "user_action",
            sessionId,
            component: "TerminalOutput",
            labels: {
                button: "dev-panel",
                state: next ? "open" : "closed"
            }
        });
        setDevGroupOpen(next);
        try {
            localStorage.setItem('stapler-squad-dev-toolbar', next ? 'true' : 'false');
        } catch  {
        // localStorage unavailable
        }
        if (!next) {
            setTimeout(()=>devToggleRef.current?.focus(), 0);
        }
    };
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        try {
            localStorage.setItem('stapler-squad-toolbar-expanded', String(toolbarExpanded));
        } catch  {
        // localStorage unavailable — continue without persistence
        }
    }, [
        toolbarExpanded
    ]);
    // Mouse tracking mode: "none" on mobile (enables xterm selection + touch scroll),
    // "any" on desktop (forwards mouse events to terminal app for vim/tmux mouse support).
    // User can toggle this via the ⌨️ toolbar to switch modes.
    const [mouseMode, setMouseMode] = (0,react.useState)('any');
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        setMouseMode(isMobile ? 'none' : 'any');
    }, [
        isMobile
    ]);
    const toggleMouseMode = (0,react.useCallback)(()=>{
        setMouseMode((prev)=>prev === 'none' ? 'any' : 'none');
    }, []);
    // Recording state
    const [isRecording, setIsRecording] = (0,react.useState)(false);
    // Opt-in, display-only output filter: reads lines out of xterm's buffer and
    // renders its own list, so it never touches the stream or the buffer itself.
    const [filterOpen, setFilterOpen] = (0,react.useState)(false);
    const getFilterLines = (0,react.useCallback)(()=>readBufferLines(xtermRef.current?.terminal), [
        xtermRef
    ]);
    // The terminal chrome (tabs, header) always uses the dark VS Code-style
    // palette defined in terminalTokens (styles/theme.css.ts), independent of
    // the app's selectable UI theme. xterm.js must match that fixed palette —
    // deriving it from prefers-color-scheme instead caused the terminal canvas
    // to render solid white whenever the OS/browser preference was light while
    // the surrounding chrome stayed dark. (Story 3: the pooled <XtermTerminal>
    // itself now hardcodes theme="dark" directly -- see TerminalPool.tsx --
    // for the same reason; no local `theme` binding is needed here anymore.)
    // Paging state for on-demand scrollback loading (Task 2.3.1 / 2.3.2)
    const isFetchingScrollbackRef = (0,react.useRef)(false);
    const hasMoreScrollbackRef = (0,react.useRef)(false);
    const oldestSequenceReceivedRef = (0,react.useRef)(0);
    // Story 1.4.0 — client-local mirror of TerminalStreamManager's altScreenActive,
    // read by the wheel listener and passed to useTerminalGestures as
    // isAltScreenActive. A ref (not state) since it's read from event-handler
    // closures, not rendered directly.
    const altScreenActiveRef = (0,react.useRef)(false);
    // Story 1.4.4 (Task 1.4.4b) — "no more app-forwarded history" flag, distinct
    // from hasMoreScrollbackRef above: the tmux-native path owns that ref
    // exclusively and this feature never reads or writes it, so a plain-shell /
    // flag-off / non-Claude session's exhaustion behavior can't be affected even
    // indirectly through shared state (plan.md Story 1.4.4 Scope correction).
    const hasMoreAppScrollbackRef = (0,react.useRef)(true);
    // Story 1.4.2 — cached from the last AppScrollbackResponse.program this
    // session has seen (any outcome), so the loading pill can show a
    // program-specific label for a *subsequent* app-forwarded request within
    // the same session even though the program name itself only ever arrives
    // in the response, never known ahead of the request that triggers it.
    const lastKnownAppProgramRef = (0,react.useRef)(undefined);
    // ---- Story 1.4.5 — scroll-forward loading pill state ----
    // Shared by BOTH the tmux-native paged-scrollback trigger and Story 1.4.0's
    // app-forwarded alt-screen triggers (design/ux.md Surface 1) — every
    // requestScrollback call site funnels through requestScrollbackWithPill
    // below so the pill can't drift out of sync with which trigger fired.
    const { pillState, clearScrollLoadingState, startScrollLoadingTimers } = useScrollLoadingPill();
    // Task 1.4.5b — Cancel resets local fetch-in-flight state and clears the
    // pill without cancelling the in-flight server request (design/ux.md: a
    // late response is simply ignored once this state has been reset).
    const handleCancelScrollLoading = (0,react.useCallback)(()=>{
        clearScrollLoadingState();
        isFetchingScrollbackRef.current = false;
    }, [
        clearScrollLoadingState
    ]);
    // ---- Story 1.4.2 — ScrollSourceIndicator banner state ----
    const [appScrollbackState, setAppScrollbackState] = (0,react.useState)({
        active: false,
        program: ""
    });
    // Ref mirror read by handleOutput (Task 1.4.2b) — kept out of handleOutput's
    // own dependency array so a banner mount/unmount can't recreate that
    // callback's identity, which useTerminalStream's connection effect depends
    // on (recreating it there would spuriously re-run connection setup).
    const appScrollbackActiveRef = (0,react.useRef)(false);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        appScrollbackActiveRef.current = appScrollbackState.active;
    }, [
        appScrollbackState.active
    ]);
    // Distinguishes the redraw echo of our own scroll-forward from a genuine live
    // resume -- see ForwardEchoGuard's doc comment for why content, not timing.
    const [echoGuard] = (0,react.useState)(()=>new ForwardEchoGuard());
    // ---- Story 1.4.3 — Blocked outcome toast state ----
    const { blockedToast, connectionPulse, showBlockedOutcome, dismissBlockedToast } = useBlockedToast();
    // ---- Story 1.4.4 — "No more history available" (AT_TOP) affordance state ----
    const [atTopVisible, setAtTopVisible] = (0,react.useState)(false);
    // Resets scrollback paging so a subsequent scroll-up re-fetches history fresh from
    // the server (which re-derives it width-agnostically via `tmux capture-pane -J`,
    // see handleScrollbackRequest in connectrpc_websocket.go) instead of assuming
    // whatever paging cursor was left over from before the terminal was last cleared.
    // Registered as TerminalStreamManager's onFullSnapshot callback below — the manager
    // itself owns detecting a full-pane replacement snapshot and clearing the xterm
    // buffer for it (ANSI_SNAPSHOT_PREFIX in TerminalStreamManager.ts); this only resets
    // the paging refs the manager doesn't know about.
    // Gesture-cancel / netPagesUp-reset trigger (ux.md S7/S9): bumped on every reconnect
    // and on every full-snapshot write.
    const [connectionEpoch, setConnectionEpoch] = (0,react.useState)(0);
    const bumpConnectionEpoch = (0,react.useCallback)(()=>setConnectionEpoch((e)=>e + 1), []);
    const resetScrollbackPaging = (0,react.useCallback)(()=>{
        hasMoreScrollbackRef.current = true;
        hasMoreAppScrollbackRef.current = true;
        oldestSequenceReceivedRef.current = 0;
        isFetchingScrollbackRef.current = false;
        // A full-pane replacement snapshot (reconnect, resize resync) means any
        // forwarded-history state from before it is now stale — design/ux.md
        // Surface 2's edge case: "reconnection resets to the live tail... a stale
        // 'Viewing history' banner never survives a reconnect."
        clearScrollLoadingState();
        setAppScrollbackState({
            active: false,
            program: ""
        });
        setAtTopVisible(false);
        dismissBlockedToast();
        echoGuard.clear();
    }, [
        clearScrollLoadingState,
        dismissBlockedToast,
        echoGuard
    ]);
    // Ref-mirror for handleAppScrollbackFrame (Story 1.4.1-1.4.4, defined further
    // below) — same temporal-dead-zone reason as notifyResyncOutputReceivedRef:
    // getOrCreateStreamManager registers it with the manager before the real
    // handler (which needs pill/banner/toast state declared later) exists.
    const handleAppScrollbackFrameRef = (0,react.useRef)(()=>{});
    // Proactively clears the terminal and resets scrollback paging before a resize RPC
    // is sent — used only by the resize triggers below (auto-fit and the manual Resize
    // button), so the screen goes blank immediately rather than showing stale,
    // differently-wrapped content for the ~100-400ms round trip until the server's
    // post-resize snapshot arrives and TerminalStreamManager clears it again anyway.
    const clearBufferBeforeResize = (0,react.useCallback)(()=>{
        clearedAtRef.current = Date.now();
        mobileDebug/* mobileDebug */.Uo.log("clear-before-resize", {});
        xtermRef.current?.clear();
        resetScrollbackPaging();
    }, [
        resetScrollbackPaging,
        xtermRef
    ]);
    // Lazily create or get the TerminalStreamManager
    const getOrCreateStreamManager = (0,react.useCallback)(()=>{
        if (streamManagerRef.current) return streamManagerRef.current;
        const terminal = xtermRef.current?.terminal;
        if (!terminal) return null;
        const manager = new TerminalStreamManager(terminal, (paused, watermark)=>sendFlowControlRef.current?.(paused, watermark));
        // Detect + clear on a full-pane replacement snapshot (ANSI_SNAPSHOT_PREFIX) — the
        // single spot this is handled; every write() call site (live output, the RESIZING
        // queue flush) benefits without needing its own check.
        manager.setOnFullSnapshot(()=>{
            resetScrollbackPaging();
            bumpConnectionEpoch();
            if (clearedAtRef.current !== null) {
                mobileDebug/* mobileDebug */.Uo.log("snapshot-after-clear", {
                    elapsedMs: Date.now() - clearedAtRef.current
                });
                clearedAtRef.current = null;
            }
        });
        // Story 1.4.0 (Task 1.4.0a) — mirror the manager's altScreenActive locally
        // so the wheel listener / useTerminalGestures can read it synchronously.
        manager.setOnAltScreenChange((active)=>{
            altScreenActiveRef.current = active;
        });
        // Story 1.4.1 (Task 1.4.1a) — route a captured AppScrollbackResponse frame
        // to this component's outcome-rendering logic, defined further below.
        // Indirected through a ref (same temporal-dead-zone reason as
        // notifyResyncOutputReceivedRef above handleOutput) since
        // handleAppScrollbackFrame is defined after getOrCreateStreamManager.
        manager.setOnAppScrollback((frame)=>handleAppScrollbackFrameRef.current(frame));
        // Inject SerializeAddon so prependScrollbackBatch can serialize the current buffer
        // before clearing it (enables correct history order without losing live content).
        const serializeAddon = xtermRef.current?.serializeAddon;
        if (serializeAddon) {
            manager.setSerializeAddon(serializeAddon);
        }
        // Track first output for metrics and loading overlay
        manager.setOnFirstOutput(()=>{
            if (metricsRef.current.firstOutputTime === null) {
                metricsRef.current.firstOutputTime = performance.now();
                logTerminalMetrics();
                const totalLoadTime = metricsRef.current.firstOutputTime - metricsRef.current.mountTime;
                track({
                    name: "session_attach",
                    category: "performance",
                    durationMs: totalLoadTime,
                    labels: {
                        phase: "attach"
                    },
                    sessionId
                });
                const connectionDuration = metricsRef.current.connectedTime && metricsRef.current.connectionInitTime ? metricsRef.current.connectedTime - metricsRef.current.connectionInitTime : totalLoadTime;
                track({
                    name: "stream_terminal_first_byte",
                    category: "performance",
                    durationMs: connectionDuration,
                    sessionId
                });
                setIsLoadingInitialContent(false);
                // Story 3 (Task 3.4) — mark this pool entry warm so a future
                // TerminalOutput remount for the same session skips the loading
                // overlay (see isLoadingInitialContent's lazy initializer above).
                warmRef.current = true;
            }
        });
        // Install debug monitoring (respects debug-terminal localStorage flag)
        manager.installDebugMonitor();
        streamManagerRef.current = manager;
        return manager;
    }, [
        logTerminalMetrics,
        sessionId,
        track,
        resetScrollbackPaging,
        bumpConnectionEpoch,
        xtermRef,
        warmRef
    ]);
    const handleAppScrollbackFrame = (0,react.useCallback)((frame)=>{
        // Task 1.4.5b — the pill never persists past outcome delivery.
        clearScrollLoadingState();
        isFetchingScrollbackRef.current = false;
        if (frame.program) {
            lastKnownAppProgramRef.current = frame.program;
        }
        switch(frame.outcome){
            case events_pb/* ScrollForwardOutcome */.tB.DELIVERED:
                {
                    const manager = getOrCreateStreamManager();
                    if (manager && frame.content) {
                        void manager.prependScrollbackBatch(frame.content);
                    }
                    appScrollbackActiveRef.current = true;
                    setAppScrollbackState({
                        active: true,
                        program: frame.program
                    });
                    dismissBlockedToast();
                    echoGuard.record(frame.content);
                    break;
                }
            case events_pb/* ScrollForwardOutcome */.tB.AT_TOP:
                {
                    // Task 1.4.4b — a distinct flag from hasMoreScrollbackRef, which
                    // this feature never reads or writes (plan.md Story 1.4.4's Scope
                    // correction). Does NOT touch appScrollbackState — a banner already
                    // showing stays up alongside this affordance (design/ux.md Surface
                    // 2's "banner does not flicker off between pages" edge case).
                    hasMoreAppScrollbackRef.current = false;
                    setAtTopVisible(true);
                    echoGuard.record(frame.content);
                    break;
                }
            case events_pb/* ScrollForwardOutcome */.tB.NO_CAPABILITY:
                break;
            case events_pb/* ScrollForwardOutcome */.tB.BLOCKED:
            default:
                showBlockedOutcome(frame);
                break;
        }
    }, [
        clearScrollLoadingState,
        getOrCreateStreamManager,
        dismissBlockedToast,
        showBlockedOutcome,
        echoGuard
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        handleAppScrollbackFrameRef.current = handleAppScrollbackFrame;
    }, [
        handleAppScrollbackFrame
    ]);
    // Story 1.4.1 (Task 1.4.1a) — decode useTerminalStream's raw
    // AppScrollbackResponse fields and dispatch through TerminalStreamManager,
    // mirroring how handleOutput routes through manager.write().
    const handleAppScrollbackReceived = (0,react.useCallback)((content, outcome, program, forwardId, blockedReason)=>{
        const manager = getOrCreateStreamManager();
        manager?.handleAppScrollback({
            content,
            outcome,
            program,
            forwardId,
            blockedReason
        });
    }, [
        getOrCreateStreamManager
    ]);
    // Story 1.4.0 bug fix — see TerminalStreamManager.setAltScreenActive()'s
    // doc comment: a capture-pane-derived initial/resize snapshot can never
    // carry the DECSET 1049h marker, so this server-authoritative hint (not
    // content-scanning) is what lets a client connecting to an already
    // alt-screen-active session learn that state at all.
    const handleAltScreenActiveHint = (0,react.useCallback)((active)=>{
        const manager = getOrCreateStreamManager();
        if (active) {
            manager?.setAltScreenActive();
        } else {
            manager?.setAltScreenInactive();
        }
    }, [
        getOrCreateStreamManager
    ]);
    // Ref to track whether the initial scrollback has been written (Task 2.3.2)
    const isInitialScrollbackDoneRef = (0,react.useRef)(false);
    // Callback to write initial pane content to terminal.
    // Metadata guard removed (R2.7): all scrollback — both initial and historical —
    // is now allowed to proceed. Initial load calls writeInitialContent(); paged
    // loads (subsequent calls with metadata) call prependScrollbackBatch().
    const handleScrollbackReceived = (0,react.useCallback)(async (scrollback, metadata)=>{
        // Task 1.4.5b — the pill never persists past outcome delivery, for any
        // outcome including the unchanged tmux-native path.
        clearScrollLoadingState();
        if (!xtermRef.current?.terminal) return;
        const manager = getOrCreateStreamManager();
        if (!manager) return;
        if (!isInitialScrollbackDoneRef.current) {
            // Initial load
            console.log(`[TerminalOutput] Writing initial scrollback: ${scrollback.length} bytes`);
            isInitialScrollbackDoneRef.current = true;
            await manager.writeInitialContent(scrollback);
            if (metadata) {
                hasMoreScrollbackRef.current = metadata.hasMore;
                oldestSequenceReceivedRef.current = metadata.oldestSequence;
            }
        } else {
            // Paged history load
            console.log(`[TerminalOutput] Writing paged scrollback: ${scrollback.length} bytes`);
            try {
                await manager.prependScrollbackBatch(scrollback);
                if (metadata) {
                    hasMoreScrollbackRef.current = metadata.hasMore;
                    oldestSequenceReceivedRef.current = metadata.oldestSequence;
                }
            } catch (err) {
                console.error('[TerminalOutput] prependScrollbackBatch failed:', err);
            } finally{
                isFetchingScrollbackRef.current = false;
            }
        }
        setIsLoadingInitialContent(false);
    }, [
        getOrCreateStreamManager,
        clearScrollLoadingState,
        xtermRef
    ]);
    // Ref to access current terminalState inside the handleOutput callback
    // (useCallback can't take terminalState as a dep without recreating on every state change)
    const terminalStateRef = (0,react.useRef)('DISCONNECTED');
    // Ref-mirror for notifyResyncOutputReceived (Story 2.1.5, Task 2.1.5b). This
    // must be a ref rather than a direct useCallback dependency: handleOutput is
    // defined here and passed into useTerminalStream as `onOutput`, but
    // notifyResyncOutputReceived isn't available until useVisibilityResync is
    // called further below (which itself depends on useTerminalStream's return
    // values) — referencing it directly in this dependency array would be a
    // temporal-dead-zone error. Same ref-mirror idiom used throughout this file
    // (e.g. sendFlowControlRef) and in useTerminalStream.ts (connectRef).
    const notifyResyncOutputReceivedRef = (0,react.useRef)(()=>{});
    // Epic 3.1, Task 3.1.2.1 — shared between useTerminalStream (which forwards
    // it to useTerminalFlowControl, where both the visibility- and
    // resize-triggered resync paths register their resync_id) and this
    // component's own handleOutput below (which checks/clears membership on
    // each incoming TerminalOutput.resync_id). Lets either resync flow's stall
    // watchdog be reset when a match arrives on ANY tracked request, without
    // merging useVisibilityResync and useTerminalFlowControl into one hook.
    //
    // Task 3.1.2.4a — a plain `Set<string>` only records *membership*, not
    // *when* an ID was added, so a resync whose own response silently hangs
    // has no way to notice it's been outstanding too long if sibling resync
    // traffic keeps resetting the shared stall watchdog. Map<resyncId, addedAtMs>
    // keeps the same membership semantics (`.has`/`.delete` used by
    // handleOutput below are unchanged) while letting useVisibilityResync's
    // resetStallWatchdog (Task 3.1.2.4b) compute a specific ID's elapsed
    // outstanding time and escalate past a 2x-timeout ceiling instead of
    // resetting indefinitely.
    const outstandingResyncIdsRef = (0,react.useRef)(new Map());
    // Ref-mirror for resetStallWatchdog (Epic 3.1, Task 3.1.2.2) — same
    // temporal-dead-zone reason as notifyResyncOutputReceivedRef above:
    // useVisibilityResync isn't called until after this handleOutput
    // definition and the useTerminalStream call that consumes it.
    const resetStallWatchdogRef = (0,react.useRef)(()=>{});
    // Callback to write output directly to terminal via TerminalStreamManager.
    // Task 4.2.2: Output is queued during RESIZING to prevent bytes at the old column width
    // from being written before the post-resize snapshot.
    const handleOutput = (0,react.useCallback)((output, resyncId)=>{
        if (!xtermRef.current) return;
        // Epic 3.1 (AC2) — if this output is the reply to a correlation-ID-
        // tagged resync request tracked via EITHER hook's resync flow (resize or
        // visibility/focus), clear it here and reconcile the visibility hook's
        // stall watchdog too, so a resize-triggered resync response doesn't
        // leave a concurrently-pending visibility resync looking stalled.
        if (resyncId && outstandingResyncIdsRef.current.has(resyncId)) {
            outstandingResyncIdsRef.current.delete(resyncId);
            resetStallWatchdogRef.current();
        }
        // Signal resync receipt regardless of whether this output is written
        // immediately or queued for post-resize flush — a concurrent resize
        // (plausible on backgrounded-tab wakeup) must not make a pending
        // visibility/focus resync look stalled just because its response
        // arrived while RESIZING.
        notifyResyncOutputReceivedRef.current(resyncId);
        // Task 1.4.2b — a normal (non-app-scrollback) output frame means the
        // server has resumed live streaming, i.e. the scroll lease was released —
        // the banner is cleared on such a frame, unless it's recognizable as the
        // redraw echo of our own last DELIVERED/AT_TOP forward -- see
        // ForwardEchoGuard's doc comment.
        if (appScrollbackActiveRef.current && !echoGuard.isSelfEcho(output)) {
            appScrollbackActiveRef.current = false;
            setAppScrollbackState({
                active: false,
                program: ""
            });
        }
        if (terminalStateRef.current === 'RESIZING') {
            pendingOutputDuringResizeRef.current.push(output);
            return;
        }
        // TerminalStreamManager.write() itself detects a full-pane replacement snapshot
        // (ANSI_SNAPSHOT_PREFIX) and clears the buffer for it — see setOnFullSnapshot in
        // getOrCreateStreamManager above.
        const manager = getOrCreateStreamManager();
        if (manager) {
            manager.write(output);
        }
    }, [
        getOrCreateStreamManager,
        echoGuard,
        xtermRef
    ]);
    // Unified WebSocket streaming (effectiveSessionId computed near the top of
    // this component -- see usePooledTerminal's call site comment for why).
    // Stable callbacks
    const getTerminal = (0,react.useCallback)(()=>xtermRef.current?.terminal || null, [
        xtermRef
    ]);
    const handleStreamError = (0,react.useCallback)((err)=>{
        console.error(`Terminal stream error (${isExternal ? 'external' : 'managed'}):`, err);
        setConnectionAttempts((prev)=>prev + 1);
        // A stream error (e.g. a malformed/aborted response while the initial
        // content fetch was in flight) doesn't necessarily flip isConnected —
        // Connect-Web's underlying WebSocket transport can stay nominally
        // connected even though this specific RPC stream errored out. Without
        // this, neither the wasConnected-&&-!isConnected disconnect handler nor
        // the connectionAttempts>=5 fallback (both further down this file) ever
        // fires for a lone error like this, leaving the user staring at an
        // infinite "Loading terminal content..." spinner with no path to the
        // reconnect banner/retry button. Clear it here too, same as the
        // disconnect branch does, so a single early error still surfaces a
        // recoverable state instead of a permanent stall.
        setIsLoadingInitialContent(false);
    }, [
        isExternal
    ]);
    // Story 2.3 — InputDropBadge: dropped-keystroke count + a monotonic
    // per-episode sequence number (so two consecutive episodes with an
    // identical count still produce a distinct announcement, see
    // InputDropBadge.tsx's `episodeSeq` doc comment).
    const [dropEpisode, setDropEpisode] = (0,react.useState)({
        count: 0,
        seq: 0
    });
    const handleDropEpisodeFlush = (0,react.useCallback)((count)=>{
        if (count <= 0) return; // design/ux.md §3.3 — defensive no-op
        setDropEpisode((prev)=>({
                count,
                seq: prev.seq + 1
            }));
    }, []);
    const reportDroppedInput = useDropEpisodeCoalescer(handleDropEpisodeFlush, DROP_EPISODE_COALESCE_WINDOW_MS);
    // Task 2.3.5 — e2e test-only trigger. Reproducing a genuine WebSocket
    // reconnect race (the real trigger for onInputDropped) inside Playwright
    // proved impractical for this pass (it would require deterministically
    // racing a server-side connection teardown against a client keystroke),
    // so tests/e2e/input-drop-badge.spec.ts exercises the badge's rendering/
    // announcement/dismiss behavior via this harmless, additive test seam
    // instead of the full reconnect path (Stories 2.1/2.2 already have direct
    // Jest coverage of the drop mechanism itself). Gated on NODE_ENV (matching
    // the existing dev-only-hook convention in WebVitalsReporter.tsx /
    // rpcTiming.ts) so this unauthenticated, script-callable surface is never
    // attached in a production bundle — Next.js statically inlines
    // `process.env.NODE_ENV` at build time, so the production build tree-shakes
    // this block out entirely rather than merely no-op'ing it at runtime.
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (true) return;
        window.__e2eTriggerInputDropped = reportDroppedInput;
        return ()=>{
            delete window.__e2eTriggerInputDropped;
        };
    }, [
        reportDroppedInput
    ]);
    const { isConnected, error, sendInput, isInputChunking, resize, connect, disconnect, scrollbackLoaded, requestScrollback, sendFlowControl, startRecording, stopRecording, terminalState, isHardFailed, handleManualReconnect: handleHookReconnect, requestFullResync, markResyncComplete, markPaneResponseReceived, connectionCount } = useTerminalStream({
        baseUrl,
        sessionId: effectiveSessionId,
        shellId,
        onShellStatusChange,
        getTerminal,
        scrollbackLines: 1000,
        autoConnect: false,
        onError: handleStreamError,
        onScrollbackReceived: handleScrollbackReceived,
        onOutput: handleOutput,
        onAppScrollback: handleAppScrollbackReceived,
        onAltScreenActiveHint: handleAltScreenActiveHint,
        initialCols: lastResizeRef.current?.cols,
        initialRows: lastResizeRef.current?.rows,
        isExternal: isExternal,
        onInputDropped: reportDroppedInput,
        foreground: isVisible,
        outstandingResyncIdsRef,
        readOnly
    });
    // Lightweight, single-purpose RPC client for the worktree-missing hard-fail
    // banner's "Retry now" button — mirrors the pattern used throughout this
    // codebase (e.g. useLogViewer.ts) for a one-off unary call, rather than
    // pulling in the full useSessionService hook (which sets up its own
    // session-list watch stream and Redux wiring — too heavy to instantiate
    // per open terminal pane).
    const retrySessionClientRef = (0,react.useRef)(null);
    if (!retrySessionClientRef.current) {
        retrySessionClientRef.current = (0,promise_client/* createClient */.UU)(session_pb/* SessionService */.Qow, (0,connect_transport/* createConnectTransport */.w)({
            baseUrl
        }));
    }
    const reconnectViaHook = (0,react.useCallback)(()=>{
        bumpConnectionEpoch();
        handleHookReconnect();
    }, [
        bumpConnectionEpoch,
        handleHookReconnect
    ]);
    const [isRetryingSession, setIsRetryingSession] = (0,react.useState)(false);
    const handleRetryNow = (0,react.useCallback)(async ()=>{
        setIsRetryingSession(true);
        try {
            // Errors are surfaced to the user via the banner remaining visible
            // (a fresh connect attempt below will just fail again) — no separate
            // error UI needed for this action itself.
            await retrySessionClientRef.current?.retrySession({
                id: effectiveSessionId
            });
        } finally{
            setIsRetryingSession(false);
            reconnectViaHook();
        }
    }, [
        effectiveSessionId,
        reconnectViaHook
    ]);
    const { notifyResyncOutputReceived, resetStallWatchdog } = useVisibilityResync({
        sessionId: effectiveSessionId,
        // TerminalOutputProps.isVisible is optional (external callers may omit
        // it entirely); useVisibilityResync's isVisible is required since its
        // visibility-scoped resync gating (Task 6.1.1.3) needs a concrete
        // boolean, not undefined. Default to false — matching the "external
        // caller opted out of visibility awareness" behavior other isVisible
        // call sites in this component already fall back to (e.g. line ~1785's
        // `isVisible !== false` treats undefined as visible for rendering, but
        // for resync-gating purposes treating "unknown" as "not visible" is the
        // safer default: it defers resync-on-focus rather than risking one
        // firing for a caller that never intended to opt in).
        isVisible: isVisible ?? false,
        isConnected,
        terminalState,
        connect,
        disconnect,
        requestFullResync,
        markResyncComplete,
        markPaneResponseReceived,
        setShowReconnectButton,
        setShowReconnectBanner,
        scheduleResync,
        outstandingResyncIdsRef
    });
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        notifyResyncOutputReceivedRef.current = notifyResyncOutputReceived;
    }, [
        notifyResyncOutputReceived
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        resetStallWatchdogRef.current = resetStallWatchdog;
    }, [
        resetStallWatchdog
    ]);
    // Sync terminalState into a ref so handleOutput can read it without recreating the callback.
    // Also flushes queued output when transitioning from RESIZING to STABLE (Task 4.2.2).
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const prevState = terminalStateRef.current;
        terminalStateRef.current = terminalState;
        if (prevState === 'RESIZING' && terminalState === 'STABLE') {
            const manager = streamManagerRef.current;
            if (manager) {
                const pending = pendingOutputDuringResizeRef.current.splice(0);
                // manager.write() itself detects and clears for a full-pane snapshot — see
                // setOnFullSnapshot in getOrCreateStreamManager above.
                for (const chunk of pending){
                    manager.write(chunk);
                }
                console.log(`[TerminalOutput] Flushed ${pending.length} pending output chunks after resize quiescence`);
            }
        }
    }, [
        terminalState
    ]);
    // Remote browser log streaming for mobile debugging
    useBrowserLogStream({
        enabled: logStreamEnabled,
        sessionId,
        baseUrl
    });
    // Update sendFlowControl ref when available + sync it into TerminalStreamManager
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        sendFlowControlRef.current = sendFlowControl;
        if (streamManagerRef.current) {
            streamManagerRef.current.updateSendFlowControl((paused, watermark)=>sendFlowControlRef.current?.(paused, watermark));
        }
    }, [
        sendFlowControl
    ]);
    // Cleanup on unmount
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        return ()=>{
            isMountedRef.current = false;
            if (pasteErrorTimerRef.current) {
                clearTimeout(pasteErrorTimerRef.current);
                pasteErrorTimerRef.current = null;
            }
            if (uploadErrorTimerRef.current) {
                clearTimeout(uploadErrorTimerRef.current);
                uploadErrorTimerRef.current = null;
            }
            if (uploadSuccessTimerRef.current) {
                clearTimeout(uploadSuccessTimerRef.current);
                uploadSuccessTimerRef.current = null;
            }
            if (sizeStabilityTimeoutRef.current) {
                clearTimeout(sizeStabilityTimeoutRef.current);
                sizeStabilityTimeoutRef.current = null;
            }
            if (bannerTimerRef.current) {
                clearTimeout(bannerTimerRef.current);
                bannerTimerRef.current = null;
            }
            // Cleanup TerminalStreamManager
            if (streamManagerRef.current) {
                streamManagerRef.current.cleanup();
                streamManagerRef.current = null;
            }
            disconnect();
        };
    }, [
        disconnect
    ]);
    // Cancel the debounced approval refresh timer on unmount to prevent post-unmount calls
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        return ()=>{
            if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
        };
    }, []);
    // TUI page-key estimate (declared early: every input path below can invalidate it).
    const [netPagesUp] = (0,react.useState)(createNetPagesUpTracker);
    // Delivers bytes to the session without touching the page-key estimate. Programmatic page
    // keys (drag, momentum, toolbar PgUp/PgDn, the jump button) use this directly; user input
    // goes through handleTerminalData, which also invalidates the estimate.
    const deliverTerminalData = (0,react.useCallback)((data)=>{
        sendInput(data);
        // Optimistic clear on Enter only — reduces false-positive flicker
        if (data === "\r") {
            clearForSession(sessionId);
        // clearForSession fires an eager refetch — skip the debounce timer for Enter
        // to avoid a second refetch 300ms later that would cause a flicker window
        } else {
            // Only debounce-refetch when there are pending approvals to update —
            // prevents RPC storm when holding keys across N open terminals
            if (pendingCount > 0) {
                if (refreshTimerRef.current) clearTimeout(refreshTimerRef.current);
                refreshTimerRef.current = setTimeout(()=>void refreshApprovals(), 300);
            }
        }
    }, [
        sendInput,
        clearForSession,
        sessionId,
        refreshApprovals,
        pendingCount
    ]);
    // User keystrokes and pastes move the app's own position, so the estimate no longer holds.
    const lastReadOnlyHintRef = (0,react.useRef)(0);
    const handleTerminalData = (0,react.useCallback)((data)=>{
        if (readOnly) {
            const now = Date.now();
            if (now - lastReadOnlyHintRef.current >= READ_ONLY_HINT_INTERVAL_MS) {
                lastReadOnlyHintRef.current = now;
                announce("Read-only session", "polite", "terminal-read-only-hint");
            }
            return;
        }
        netPagesUp.invalidate("keystroke");
        deliverTerminalData(data);
    }, [
        netPagesUp,
        deliverTerminalData,
        readOnly,
        announce
    ]);
    // Scroll route shared by the toolbar PgUp/PgDn (and later the chip and jump button).
    // XtermTerminal reports the live mode upward only when it changes.
    const { override: scrollOverride, gestureScrollEnabled } = useScrollSettings();
    const [scrollMode, setScrollMode] = (0,react.useState)(INITIAL_SCROLL_MODE);
    const handleScrollModeChange = (0,react.useCallback)((next)=>{
        setScrollMode((prev)=>prev.bufferType === next.bufferType && prev.mouseTrackingMode === next.mouseTrackingMode ? prev : next);
    }, []);
    const effectiveScroll = useEffectiveScrollMode(scrollMode, scrollOverride, gestureScrollEnabled);
    // Scrolling chip / picker / full panel (ux.md S6). Rows come from the terminal's own resize report.
    const [scrollPanel, setScrollPanel] = (0,react.useState)(null);
    const [terminalRows, setTerminalRows] = (0,react.useState)(0);
    // The xterm instance is created inside XtermTerminal's effect; the first resize report reveals it.
    const [jumpTerminal, setJumpTerminal] = (0,react.useState)(null);
    const [gestureActive, setGestureActive] = (0,react.useState)(false);
    // Host-owned live region: the picker unmounts on select, so its own announcement would be lost.
    const [scrollAnnouncement, setScrollAnnouncement] = (0,react.useState)("");
    const scrollChipRef = (0,react.useRef)(null);
    const misrouteCue = useMisrouteCue();
    const scrollHint = useScrollHint();
    const notifyScrollStart = scrollHint.notifyScrollStart;
    const markScrollHintSeen = scrollHint.markSeenOnPanelOpen;
    const getTouchSurface = (0,react.useCallback)(()=>terminalContainerRef.current, []);
    const reportMisroute = misrouteCue.report;
    // Drag/momentum page keys keep the estimate current instead of invalidating it.
    const handlePageKeysSent = (0,react.useCallback)((direction, pages)=>{
        for(let i = 0; i < pages; i++){
            if (direction === "up") netPagesUp.pageUp();
            else netPagesUp.pageDown();
        }
    }, [
        netPagesUp
    ]);
    const scrollGestureProps = (0,react.useMemo)(()=>({
            scrollOverride,
            gestureScrollEnabled,
            connectionEpoch,
            onScrollGesture: reportMisroute,
            onScrollStart: notifyScrollStart,
            onPageKeysSent: handlePageKeysSent,
            onGestureActiveChange: setGestureActive,
            onProgrammaticData: deliverTerminalData,
            isInputBusy: isInputChunking,
            mouseModeOverride: mouseMode
        }), [
        mouseMode,
        scrollOverride,
        gestureScrollEnabled,
        connectionEpoch,
        reportMisroute,
        notifyScrollStart,
        handlePageKeysSent,
        deliverTerminalData,
        isInputChunking
    ]);
    const handleScrollOverrideChange = (0,react.useCallback)((value)=>{
        scrollSettings.setOverride(value);
        if (scrollPanel === "picker") {
            const label = SCROLL_OPTIONS.find((o)=>o.value === value)?.announce ?? value;
            setScrollAnnouncement(`Scroll mode: ${label}`);
        }
    }, [
        scrollPanel
    ]);
    const closeScrollPanel = (0,react.useCallback)(()=>setScrollPanel(null), []);
    const openScrollPicker = (0,react.useCallback)(()=>{
        markScrollHintSeen();
        setScrollPanel((p)=>p === "picker" ? null : "picker");
    }, [
        markScrollHintSeen
    ]);
    const toggleScrollFullPanel = (0,react.useCallback)(()=>{
        markScrollHintSeen();
        setScrollPanel((p)=>p === "full" ? null : "full");
    }, [
        markScrollHintSeen
    ]);
    const openScrollFullPanel = (0,react.useCallback)(()=>setScrollPanel("full"), []);
    // Approximate panel heights in terminal rows (picker: 3 radios + 2 buttons; full: adds switch and notes).
    // Only used to decide inline vs overlay, so a rough constant is enough.
    // terminalRows is re-measured after an inline panel shrinks the terminal, so the choice is latched when the
    // panel opens (reset on close); re-deciding from the shrunken count flip-flops. The latch only upgrades
    // inline -> overlay (e.g. the soft keyboard leaves too few rows); it never downgrades while open.
    const panelOverlayLatchRef = (0,react.useRef)(null);
    const panelRowsEstimate = scrollPanel === "full" ? SCROLL_FULL_PANEL_ROWS : SCROLL_PICKER_PANEL_ROWS;
    if (scrollPanel === null) panelOverlayLatchRef.current = null;
    else if (panelOverlayLatchRef.current === null) {
        panelOverlayLatchRef.current = shouldRenderPanelAsOverlay(terminalRows - panelRowsEstimate);
    } else if (!panelOverlayLatchRef.current && shouldRenderPanelAsOverlay(terminalRows)) {
        // An inline panel has already taken its rows out of terminalRows, so no estimate is subtracted.
        panelOverlayLatchRef.current = true;
    }
    const panelAsOverlay = panelOverlayLatchRef.current ?? false;
    const panelMaxHeight = panelAsOverlay && terminalRows > MIN_ROWS_FOR_OVERLAYS && containerSize.height > 0 ? Math.round(containerSize.height * (terminalRows - MIN_ROWS_FOR_OVERLAYS) / terminalRows) : undefined;
    const lastEpochRef = (0,react.useRef)(connectionEpoch);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (lastEpochRef.current === connectionEpoch) return;
        lastEpochRef.current = connectionEpoch;
        netPagesUp.invalidate("reconnect");
    }, [
        connectionEpoch,
        netPagesUp
    ]);
    // A buffer switch, mouse-mode change or override change reroutes scrolling; skip the initial mount.
    const modeKey = `${scrollMode.bufferType}|${scrollMode.mouseTrackingMode}|${scrollOverride}`;
    const lastModeKeyRef = (0,react.useRef)(modeKey);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (lastModeKeyRef.current === modeKey) return;
        lastModeKeyRef.current = modeKey;
        netPagesUp.invalidate("mode-change");
    }, [
        modeKey,
        netPagesUp
    ]);
    // Send a key sequence, applying any active sticky modifier (CTRL or ALT) first.
    // Modifier sequences follow xterm's parameter convention:
    //   modifier 3 = Alt (escape prefix or CSI param ;3)
    //   modifier 5 = Ctrl (CSI param ;5)
    const sendKey = (0,react.useCallback)((keyData, programmatic = false)=>{
        let data = keyData;
        if (ctrlActive) {
            data = CTRL_KEY_MAP[keyData] ?? keyData;
            setCtrlActive(false);
        } else if (altActive) {
            data = ALT_KEY_MAP[keyData] ?? '\x1b' + keyData;
            setAltActive(false);
        } else if (shiftActive) {
            data = SHIFT_KEY_MAP[keyData] ?? keyData;
            setShiftActive(false);
        }
        (programmatic ? deliverTerminalData : handleTerminalData)(data);
    }, [
        ctrlActive,
        altActive,
        shiftActive,
        handleTerminalData,
        deliverTerminalData
    ]);
    // Route-aware PgUp/PgDn (ux.md S4): the same route as the drag, so the keys are an
    // equivalent single-pointer alternative. TUI-route bytes are unchanged from before.
    const sendToolbarPageKey = (0,react.useCallback)((direction)=>{
        mobileDebug/* mobileDebug */.Uo.toolbarKey(direction === "up" ? "PageUp" : "PageDown");
        const terminal = xtermRef.current?.terminal ?? null;
        const buffer = terminal?.buffer?.active;
        const canScroll = !!buffer && (direction === "up" ? buffer.viewportY > 0 : buffer.viewportY < buffer.baseY);
        const action = (0,scrollRouting/* toolbarPageAction */.d4)({
            route: effectiveScroll.target,
            direction,
            modifiers: {
                ctrl: ctrlActive,
                alt: altActive,
                shift: shiftActive
            },
            canScroll,
            override: scrollOverride
        });
        if (action.type === "scroll-pages" && terminal) {
            terminal.scrollPages(action.pages);
            return;
        }
        // A chunked paste is still streaming: page-key bytes would splice into it, so drop the tap (as the drag does).
        if (isInputChunking()) return;
        const bytes = action.type === "send-keys" ? action.bytes : direction === "up" ? scrollRouting/* PAGE_UP_BYTES */.UU : scrollRouting/* PAGE_DOWN_BYTES */.Nl;
        const countsTowardNetPages = action.type === "send-keys" && action.countsTowardNetPages;
        if (countsTowardNetPages) {
            if (direction === "up") netPagesUp.pageUp();
            else netPagesUp.pageDown();
        }
        sendKey(bytes, countsTowardNetPages);
    }, [
        effectiveScroll.target,
        scrollOverride,
        ctrlActive,
        altActive,
        shiftActive,
        sendKey,
        netPagesUp,
        isInputChunking
    ]);
    // Handle terminal resize with size stability detection
    const sizingRefs = {
        lastResizeRef,
        hasInitiatedConnectionRef,
        hasCachedDimensionsRef,
        sizeStabilityTimeoutRef,
        pendingConnectAfterDisconnectRef,
        isMountedRef,
        metricsRef,
        settleBypassPendingRef
    };
    const onTerminalResized = (0,react.useCallback)((_cols, rows)=>{
        setTerminalRows(rows);
        setJumpTerminal(xtermRef.current?.terminal ?? null);
        netPagesUp.invalidate("resize");
    }, [
        netPagesUp,
        xtermRef
    ]);
    const handleTerminalResize = useTerminalResizeHandler({
        sessionId,
        xtermRef,
        isConnected,
        error,
        connect,
        resize,
        clearBufferBeforeResize,
        setIsWaitingForStableSize,
        onTerminalResized,
        refs: sizingRefs
    });
    // Monitor connection state changes
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const wasConnected = previousConnectionStateRef.current;
        previousConnectionStateRef.current = isConnected;
        let postConnectionResizeTimer = null;
        if (!wasConnected && isConnected) {
            if (metricsRef.current.connectedTime === null) {
                metricsRef.current.connectedTime = performance.now();
            }
            console.log("[TerminalOutput] Connection established");
            setShowReconnectButton(false);
            setConnectionAttempts(0);
            // On reconnect (not first connect), append a separator so the user can see where
            // the reconnection happened. showReconnectBanner being true means we disconnected.
            if (hasEverConnectedRef.current && showReconnectBanner) {
                const terminal = xtermRef.current?.terminal;
                if (terminal && terminal.buffer.active.length > 0) {
                    const manager = streamManagerRef.current;
                    if (manager) {
                        manager.write("\r\n\x1b[2m--- reconnected ---\x1b[0m\r\n");
                    }
                }
            }
            if (reconnectTimeoutRef.current) {
                clearTimeout(reconnectTimeoutRef.current);
                reconnectTimeoutRef.current = null;
            }
            // Delay the post-connection resize sync until after layout settles.
            // Firing immediately would start the 200ms throttle clock at T+0, causing
            // the ResizeObserver's settled-layout resize (arriving at T+150ms via its
            // own debounce) to be dropped. Waiting 250ms lets the container stabilise
            // first; by then the ResizeObserver has already sent the correct dims (or
            // nothing changed and we send here as a safety net).
            //
            // force=true (AC4): reconnect must re-sync the server's pane content even
            // when the settled dims exactly match the last value resize()'s RPC-level
            // dedup would otherwise skip the send — a dimension-unchanged reconnect
            // still needs the follow-up currentPaneRequest that only resize() sends.
            postConnectionResizeTimer = setTimeout(()=>{
                const settledSize = lastResizeRef.current;
                if (settledSize) {
                    console.log(`[TerminalOutput] Post-connection resize sync (delayed, force=true): ${settledSize.cols}x${settledSize.rows}`);
                    resize(settledSize.cols, settledSize.rows, true);
                }
            }, 250);
        } else if (wasConnected && !isConnected) {
            console.log("[TerminalOutput] Connection lost, will attempt reconnection");
            // New session: don't drop the spinner before first content arrives.
            if (isInitialScrollbackDoneRef.current) {
                setIsLoadingInitialContent(false);
            }
            if (process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true") {
                reconnectTimeoutRef.current = setTimeout(()=>{
                    if (!isConnected) {
                        setShowReconnectButton(true);
                    }
                }, 5000);
            }
        }
        return ()=>{
            if (postConnectionResizeTimer) {
                clearTimeout(postConnectionResizeTimer);
            }
            if (reconnectTimeoutRef.current) {
                clearTimeout(reconnectTimeoutRef.current);
            }
        };
    // showReconnectBanner is a pre-existing missing dependency: adding it
    // would re-run this effect on every banner toggle, and its cleanup
    // clears reconnectTimeoutRef.current without nulling the ref -- that
    // would silently cancel the "show reconnect button after 5s" timer
    // mid-countdown. Left alone rather than fixed as a drive-by.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [
        isConnected,
        resize,
        xtermRef
    ]);
    // Story 3.2.1 — reconnecting banner: show after 2s of disconnection (if was ever connected)
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (isConnected) {
            hasEverConnectedRef.current = true;
            if (bannerTimerRef.current) {
                clearTimeout(bannerTimerRef.current);
                bannerTimerRef.current = null;
            }
            setShowReconnectBanner(false);
            return;
        }
        if (!hasEverConnectedRef.current) return; // never connected — don't show banner
        bannerTimerRef.current = setTimeout(()=>{
            if (!isConnected) {
                setShowReconnectBanner(true);
            }
        }, 2000);
        return ()=>{
            if (bannerTimerRef.current) {
                clearTimeout(bannerTimerRef.current);
                bannerTimerRef.current = null;
            }
        };
    }, [
        isConnected
    ]);
    // Clear loading overlay when max reconnect attempts reached
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (connectionAttempts >= 5) {
            setIsLoadingInitialContent(false);
        }
    }, [
        connectionAttempts
    ]);
    // Scroll to bottom whenever loading finishes (session open or switch)
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (!isLoadingInitialContent && xtermRef.current?.terminal) {
            const terminal = xtermRef.current.terminal;
            requestAnimationFrame(()=>{
                terminal.scrollToBottom();
                setTimeout(()=>terminal.scrollToBottom(), 100);
            });
        }
    }, [
        isLoadingInitialContent,
        xtermRef
    ]);
    // Task 1.4.5b — every requestScrollback call site (the tmux-native trigger
    // below and Story 1.4.0's alt-screen wheel/touch triggers) funnels through
    // this one wrapper so the loading pill can't drift out of sync with which
    // trigger actually fired.
    const requestScrollbackWithPill = (0,react.useCallback)((fromSequence, limit)=>{
        startScrollLoadingTimers(altScreenActiveRef.current ? lastKnownAppProgramRef.current : undefined);
        requestScrollback(fromSequence, limit);
    }, [
        requestScrollback,
        startScrollLoadingTimers
    ]);
    // Story 1.4.0 (Task 1.4.0c) — shared guard-and-call body for the alt-screen
    // wheel (Task 1.4.0b) and touch (via useTerminalGestures) triggers, so the
    // two paths can't drift apart. For an alt-screen session the "more
    // available" guard is hasMoreAppScrollbackRef (Task 1.4.4b) — this feature's
    // own flag — never hasMoreScrollbackRef, which a pure alt-screen session's
    // tmux-native path never touches and so would never flip false.
    const triggerAltScreenScrollUp = (0,react.useCallback)(()=>{
        if (isFetchingScrollbackRef.current || !hasMoreAppScrollbackRef.current || !isConnected) {
            return;
        }
        isFetchingScrollbackRef.current = true;
        requestScrollbackWithPill(oldestSequenceReceivedRef.current, 500);
    }, [
        isConnected,
        requestScrollbackWithPill
    ]);
    // Stable accessor for useTerminalGestures's isAltScreenActive option
    // (Task 1.4.0c) — reads the same ref the wheel listener above holds.
    const isAltScreenActive = (0,react.useCallback)(()=>altScreenActiveRef.current, []);
    // useTerminalGestures calls this with a line count (Task 1.4.0c); the
    // trigger itself doesn't scale by drag magnitude, matching the wheel
    // trigger's deltaY-direction-only behavior.
    const onAltScreenScrollUp = (0,react.useCallback)(()=>triggerAltScreenScrollUp(), [
        triggerAltScreenScrollUp
    ]);
    // Story 3 (Task 3.4) — part 2 of usePooledTerminal (see that hook's call
    // site near the top of this component and its own doc comment for why
    // this is a separate call, made here once all four callbacks it needs
    // exist).
    (0,TerminalPool/* usePooledTerminalCallbacks */.RW)(effectiveSessionId, isVisible !== false, {
        onData: handleTerminalData,
        onResize: handleTerminalResize,
        isAltScreenActive,
        onAltScreenScrollUp,
        scrollGesture: scrollGestureProps,
        onScrollModeChange: handleScrollModeChange
    });
    // Read-only view: xterm itself stops emitting keystrokes. handleTerminalData and the flow-control
    // hook drop anything that still gets through, and the server drops it again (Story 5.1).
    // The pooled xterm mounts lazily, so this re-checks on every render (a cheap property compare).
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        const options = xtermRef.current?.terminal?.options;
        if (options && options.disableStdin !== readOnly) options.disableStdin = readOnly;
    });
    // Task 2.3.1 — DOM scroll listener to detect near-top-of-buffer and trigger paged history load.
    // Uses DOM 'scroll' event on terminal.element (NOT terminal.onScroll, which only fires on buffer
    // writes, not user scroll gestures — see xterm.js issues #3201, #3864).
    // Task 1.4.0b — a `wheel` listener lives in this same effect: alt-screen
    // sessions (the case this whole feature exists for) never move viewportY,
    // so the 'scroll' listener above never fires for them (research/architecture.md §3).
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (isLoadingInitialContent) return; // Don't attach until initial content is loaded
        const terminal = xtermRef.current?.terminal;
        if (!terminal?.element) return;
        // xterm.js scrolls the .xterm-viewport child element, not the root terminal.element —
        // attaching the listener to terminal.element would never fire on scroll (Bug 5 fix).
        const scrollEl = terminal.element?.querySelector('.xterm-viewport') ?? terminal.element;
        const onScroll = ()=>{
            const viewportY = terminal.buffer.active.viewportY;
            if (viewportY < 200 && !isFetchingScrollbackRef.current && hasMoreScrollbackRef.current && isConnected) {
                isFetchingScrollbackRef.current = true;
                console.log(`[TerminalOutput] Near top of buffer (viewportY=${viewportY}), requesting older scrollback from seq ${oldestSequenceReceivedRef.current}`);
                requestScrollbackWithPill(oldestSequenceReceivedRef.current, 500);
            }
        };
        // Task 1.4.0b — when altScreenActive, viewportY-gated 'scroll' never fires
        // (or is meaningless), so a wheel scroll-up gesture is the trigger instead.
        // No conflict with existing handlers: nothing else registers a 'wheel'
        // listener on this element (confirmed by grep across XtermTerminal.tsx).
        const onWheel = (e)=>{
            const deltaY = e.deltaY;
            if (altScreenActiveRef.current && deltaY < 0) {
                triggerAltScreenScrollUp();
            }
        };
        scrollEl?.addEventListener('scroll', onScroll, {
            passive: true
        });
        scrollEl?.addEventListener('wheel', onWheel, {
            passive: true
        });
        return ()=>{
            scrollEl?.removeEventListener('scroll', onScroll);
            scrollEl?.removeEventListener('wheel', onWheel);
        };
    }, [
        isLoadingInitialContent,
        isConnected,
        requestScrollbackWithPill,
        triggerAltScreenScrollUp,
        xtermRef
    ]);
    // Auto-reconnect with exponential backoff
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (process.env.NEXT_PUBLIC_RECONNECT_V2 === "true") return; // hook-level reconnect handles it
        // Must not fire once the hook has hard-failed (e.g. a non-retriable WS
        // close code): the hook already set shouldReconnectRef.current = false
        // in that case, but calling connect() below unconditionally resets it
        // back to true (see useTerminalStream.ts's connect()), silently
        // reconnecting right through the hard-fail and hiding the Retry banner.
        if (isHardFailed) return;
        if (!isConnected && error && connectionAttempts > 0 && connectionAttempts < 5) {
            const backoffDelay = Math.min(1000 * Math.pow(2, connectionAttempts - 1), 10000);
            console.log(`[TerminalOutput] Auto-reconnecting in ${backoffDelay}ms (attempt ${connectionAttempts})`);
            const timeout = setTimeout(()=>{
                console.log("[TerminalOutput] Attempting reconnection...");
                bumpConnectionEpoch();
                connect();
            }, backoffDelay);
            return ()=>clearTimeout(timeout);
        }
    }, [
        isConnected,
        error,
        connectionAttempts,
        connect,
        isHardFailed,
        bumpConnectionEpoch
    ]);
    // Starts the connection for a session: pre-size from cache, then session-switch
    // connect, then post-disconnect connect. The effects inside run in that fixed
    // order -- see the ORDERING INVARIANT in useSessionConnectionBootstrap.
    useSessionConnectionBootstrap({
        sessionId,
        containerWidth: containerSize.width,
        terminalContainerRef,
        xtermRef,
        isConnected,
        connect,
        setIsWaitingForStableSize,
        setIsLoadingInitialContent,
        setConnectionAttempts,
        setShowReconnectButton,
        reconnectTimeoutRef,
        previousConnectionStateRef,
        streamManagerRef,
        refs: sizingRefs
    });
    // When terminal becomes visible (e.g. session switch in pool), refit through the sampler + focus.
    // Story 3 (Task 3.5) — usePooledTerminal's docking effect also fits+focuses on dock; this is a
    // harmless second attempt (both idempotent).
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        if (isVisible && xtermRef.current) {
            xtermRef.current.refit({
                reason: 'visibility'
            });
            xtermRef.current.terminal?.focus();
        }
    }, [
        isVisible,
        xtermRef
    ]);
    // On-screen keyboard / URL bar: refit once visualViewport height and offsetTop stop changing.
    // Sole viewport-driven fit path; the sampler in XtermTerminal owns the actual fit
    // (ADR-002: a direct fit() off visualViewport raced that sampler, so we only call refit()).
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        return createViewportSettle(window.visualViewport, createRafScheduler(), {
            stableFrames: DEFAULT_STABLE_FRAMES,
            maxWaitMs: DEFAULT_MAX_WAIT_MS,
            onSettled: (snapshot)=>{
                mobileDebug/* mobileDebug */.Uo.log("vp-settle", snapshot);
                netPagesUp.invalidate("resize");
                const xterm = xtermRef.current;
                if (!xterm) return; // no refit issued, so no bypass to arm
                settleBypassPendingRef.current = true;
                xterm.refit({
                    reason: 'viewport-settle',
                    // The fit ended without a size change (or exhausted retries): drop the unused bypass.
                    onFitted: ()=>{
                        settleBypassPendingRef.current = false;
                    }
                });
            }
        });
    }, [
        netPagesUp
    ]);
    const handleManualReconnect = (0,react.useCallback)(()=>{
        console.log("[TerminalOutput] Manual reconnect requested");
        setConnectionAttempts(0);
        setShowReconnectButton(false);
        bumpConnectionEpoch();
        connect();
    }, [
        connect,
        bumpConnectionEpoch
    ]);
    const handleToggleDebug = (0,react.useCallback)(()=>{
        const newDebugMode = !debugMode;
        setDebugMode(newDebugMode);
        if (false) {}
    }, [
        debugMode
    ]);
    const handleToggleLogStream = (0,react.useCallback)(()=>{
        const next = !logStreamEnabled;
        setLogStreamEnabled(next);
        if (false) {}
    }, [
        logStreamEnabled
    ]);
    const handleCopyOutput = ()=>{
        const selectedText = xtermRef.current?.terminal?.getSelection();
        if (selectedText) {
            navigator.clipboard.writeText(selectedText).catch(()=>{
                document.execCommand('copy');
            });
        }
    };
    // Read from clipboard and inject into the terminal.
    // Text is sent directly; images are uploaded to the server and the resulting
    // file path is inserted so the terminal process (e.g. Claude Code) can read
    // the image from disk via normal file path conventions.
    const handlePaste = (0,react.useCallback)(async ()=>{
        try {
            // navigator.clipboard.read() supports both text and image items
            if (navigator.clipboard?.read) {
                const items = await navigator.clipboard.read();
                for (const item of items){
                    const imageType = item.types.find((t)=>t.startsWith('image/'));
                    if (imageType) {
                        const blob = await item.getType(imageType);
                        // Convert to base64 (strip the "data:image/...;base64," prefix)
                        const base64 = await new Promise((resolve, reject)=>{
                            const reader = new FileReader();
                            reader.onload = ()=>resolve(reader.result.split(',')[1]);
                            reader.onerror = reject;
                            reader.readAsDataURL(blob);
                        });
                        const uploadUrl = `${baseUrl}/upload/file`;
                        const resp = await fetch(uploadUrl, {
                            method: 'POST',
                            headers: {
                                'Content-Type': 'application/json'
                            },
                            body: JSON.stringify({
                                data: base64,
                                contentType: imageType
                            })
                        });
                        if (resp.ok) {
                            const { path } = await resp.json();
                            handleTerminalData(path);
                        } else {
                            const msg = await resp.text().catch(()=>`HTTP ${resp.status}`);
                            console.warn('[TerminalOutput] Image upload failed:', msg);
                            if (pasteErrorTimerRef.current) clearTimeout(pasteErrorTimerRef.current);
                            setPasteError('Upload failed');
                            pasteErrorTimerRef.current = setTimeout(()=>setPasteError(null), 2500);
                        }
                        return;
                    }
                    if (item.types.includes('text/plain')) {
                        const blob = await item.getType('text/plain');
                        const text = await blob.text();
                        if (text) handleTerminalData(text);
                        return;
                    }
                }
            } else {
                // Fallback: text-only clipboard API
                const text = await navigator.clipboard.readText();
                if (text) handleTerminalData(text);
            }
        } catch (err) {
            console.warn('[TerminalOutput] Clipboard access failed:', err);
            if (pasteErrorTimerRef.current) clearTimeout(pasteErrorTimerRef.current);
            setPasteError('Clipboard access denied');
            pasteErrorTimerRef.current = setTimeout(()=>setPasteError(null), 2500);
        }
    }, [
        handleTerminalData,
        baseUrl
    ]);
    const handleScrollToBottom = ()=>{
        if (xtermRef.current?.terminal) {
            xtermRef.current.terminal.scrollToBottom();
        }
    };
    // Upload a batch of files sequentially, inserting each path into the terminal.
    // Shared by the file-picker inputs and drag-and-drop.
    const uploadFileList = (0,react.useCallback)(async (fileArray)=>{
        const total = fileArray.length;
        setUploadingCount(total);
        setUploadError(null);
        setUploadSuccess(null);
        let successCount = 0;
        let lastUploadedPath = "";
        for(let i = 0; i < total; i++){
            const file = fileArray[i];
            // Update count as we progress through the batch.
            setUploadingCount(total - i);
            try {
                const formData = new FormData();
                formData.append("session_id", sessionId);
                formData.append("file", file);
                // Do NOT set Content-Type header — browser sets it with multipart boundary.
                const resp = await fetch(`${baseUrl}/v1/upload-image`, {
                    method: "POST",
                    body: formData
                });
                if (resp.ok) {
                    const data = await resp.json();
                    // Always attempt to insert — sendInput silently no-ops when disconnected.
                    handleTerminalData(data.path + " ");
                    successCount++;
                    lastUploadedPath = data.path;
                } else {
                    let msg = "Upload failed";
                    if (resp.status === 413) msg = "File too large (max 10 MB)";
                    else if (resp.status === 404) msg = "Session not found";
                    setUploadError(msg);
                    if (uploadErrorTimerRef.current) clearTimeout(uploadErrorTimerRef.current);
                    uploadErrorTimerRef.current = setTimeout(()=>setUploadError(null), 3000);
                    break;
                }
            } catch  {
                setUploadError("Network error");
                if (uploadErrorTimerRef.current) clearTimeout(uploadErrorTimerRef.current);
                uploadErrorTimerRef.current = setTimeout(()=>setUploadError(null), 3000);
                break;
            }
        }
        setUploadingCount(0);
        if (successCount > 0) {
            // If terminal is disconnected, the path silently dropped — copy it to clipboard
            // and show the filename so the user can see what was uploaded.
            if (!isConnected && lastUploadedPath) {
                navigator.clipboard?.writeText(lastUploadedPath).catch(()=>{});
            }
            const label = successCount === 1 && lastUploadedPath ? lastUploadedPath.split("/").pop() ?? "file" : `${successCount} files`;
            const msg = isConnected ? `✓ ${label} inserted` : `✓ ${label} saved — path copied`;
            setUploadSuccess(msg);
            if (uploadSuccessTimerRef.current) clearTimeout(uploadSuccessTimerRef.current);
            uploadSuccessTimerRef.current = setTimeout(()=>setUploadSuccess(null), 4000);
        }
    }, [
        sessionId,
        baseUrl,
        handleTerminalData,
        isConnected
    ]);
    const handleFilesUpload = (0,react.useCallback)((e)=>{
        const fileList = e.target.files;
        if (!fileList || fileList.length === 0) return;
        // Extract File objects before resetting the input — iOS Safari invalidates FileList
        // items after e.target.value = "" because it uses lazy references.
        const fileArray = Array.from(fileList);
        // Reset input so the same file(s) can trigger onChange again.
        e.target.value = "";
        void uploadFileList(fileArray);
    }, [
        uploadFileList
    ]);
    // Drag-and-drop of OS files onto the terminal. Desktop-only and not for read-only sessions.
    const canAcceptDrop = !readOnly && hasFinePointer;
    const { isDragOver, dropHandlers } = useFileDrop(canAcceptDrop, uploadFileList);
    // Synchronous click handlers — MUST NOT have await before .click() (iOS Safari requirement).
    const handleCameraButtonClick = (0,react.useCallback)(()=>{
        cameraInputRef.current?.click();
    }, []);
    const handleGalleryButtonClick = (0,react.useCallback)(()=>{
        galleryInputRef.current?.click();
    }, []);
    const handleFilesButtonClick = (0,react.useCallback)(()=>{
        filesInputRef.current?.click();
    }, []);
    const handleClear = ()=>{
        if (xtermRef.current && xtermRef.current.terminal) {
            const terminal = xtermRef.current.terminal;
            const startTime = performance.now();
            refreshCountRef.current++;
            console.log('[TerminalOutput] Clear requested', {
                refreshCount: refreshCountRef.current,
                bufferSize: terminal.buffer.active.length,
                rows: terminal.rows,
                cols: terminal.cols,
                scrollbackSize: terminal.buffer.normal.length
            });
            xtermRef.current.clear();
            const clearTime = performance.now();
            terminal.refresh(0, terminal.rows - 1);
            const refreshTime = performance.now();
            terminal.write('\x1b[H');
            const cursorResetTime = performance.now();
            console.log('[TerminalOutput] Clear completed with forced refresh', {
                refreshCount: refreshCountRef.current,
                clearDuration: `${(clearTime - startTime).toFixed(2)}ms`,
                refreshDuration: `${(refreshTime - clearTime).toFixed(2)}ms`,
                cursorResetDuration: `${(cursorResetTime - refreshTime).toFixed(2)}ms`,
                totalDuration: `${(cursorResetTime - startTime).toFixed(2)}ms`,
                refreshedRows: `0-${terminal.rows - 1}`,
                viewport: {
                    rows: terminal.rows,
                    cols: terminal.cols,
                    scrollTop: terminal.buffer.active.viewportY
                }
            });
        }
    };
    // The fit completes asynchronously (sampler, rAF), so the forced resize message is sent from
    // onFitted with the post-fit dims. The latest connection state is read when it fires, not when
    // the button was tapped.
    const manualResizeLatestRef = (0,react.useRef)({
        isConnected,
        resize,
        clearBufferBeforeResize
    });
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react.useEffect)(()=>{
        manualResizeLatestRef.current = {
            isConnected,
            resize,
            clearBufferBeforeResize
        };
    });
    const handleManualResize = ()=>{
        console.log("[TerminalOutput] Manual resize triggered");
        xtermRef.current?.refit({
            reason: 'manual-resize',
            onFitted: ({ cols, rows, stale })=>{
                const latest = manualResizeLatestRef.current;
                // Forcing a resize (which also clears the local buffer) with pre-hide dims would redraw at a size the container no longer has.
                if (stale) {
                    console.log(`[TerminalOutput] Manual resize skipped: container never became visible (stale ${cols}x${rows})`);
                    return;
                }
                console.log(`[TerminalOutput] Terminal resized to ${cols}x${rows}`);
                if (!isMountedRef.current || !latest.isConnected) return;
                console.log(`[TerminalOutput] Forcing resize message to backend: ${cols}x${rows}`);
                lastResizeRef.current = {
                    cols,
                    rows
                };
                latest.clearBufferBeforeResize();
                latest.resize(cols, rows, true);
            }
        });
    };
    // Epic 4.2, Story 4.2.2 (Task 4.2.2b) — best-effort resize-mismatch signal
    // for ConnectionCountIndicator's tooltip. No wire field carries "did this
    // tab's ResizeVote win the hub's negotiation" (out of this UI-only epic's
    // scope), so this compares what this tab last asked to resize to
    // (lastResizeRef, set in handleTerminalResize) against xterm's actual
    // applied dimensions — if they differ, another connection's vote is
    // constraining this tab's pane size. Never shown speculatively: both refs
    // must have resolved values before a mismatch is reported (UX-AC-10).
    const appliedTerminal = xtermRef.current?.terminal;
    const hasResizeSizeMismatch = !!(lastResizeRef.current && appliedTerminal && (appliedTerminal.cols !== lastResizeRef.current.cols || appliedTerminal.rows !== lastResizeRef.current.rows));
    const allSecondaryActions = [
        {
            key: 'copy',
            icon: '📋',
            label: 'Copy',
            ariaLabel: 'Copy terminal output to clipboard',
            title: 'Copy selected terminal text to clipboard',
            extraClass: '',
            handler: ()=>{
                track({
                    name: "toolbar_button_click",
                    category: "user_action",
                    sessionId,
                    component: "TerminalOutput",
                    labels: {
                        button: "copy"
                    }
                });
                handleCopyOutput();
            }
        },
        {
            key: 'paste',
            icon: '📎',
            label: 'Paste',
            ariaLabel: 'Paste from clipboard',
            title: 'Paste from clipboard',
            extraClass: undefined,
            handler: ()=>{
                track({
                    name: "toolbar_button_click",
                    category: "user_action",
                    sessionId,
                    component: "TerminalOutput",
                    labels: {
                        button: "paste"
                    }
                });
                void handlePaste();
            }
        },
        {
            key: 'bottom',
            icon: '↓',
            label: 'Bottom',
            ariaLabel: 'Scroll to bottom',
            title: 'Scroll to bottom',
            extraClass: '',
            handler: ()=>{
                track({
                    name: "toolbar_button_click",
                    category: "user_action",
                    sessionId,
                    component: "TerminalOutput",
                    labels: {
                        button: "bottom"
                    }
                });
                handleScrollToBottom();
            }
        },
        {
            key: 'clear',
            icon: '🗑️',
            label: 'Clear',
            ariaLabel: 'Clear terminal',
            title: 'Clear terminal',
            extraClass: '',
            handler: ()=>{
                track({
                    name: "toolbar_button_click",
                    category: "user_action",
                    sessionId,
                    component: "TerminalOutput",
                    labels: {
                        button: "clear"
                    }
                });
                handleClear();
            }
        },
        {
            key: 'scrolling',
            icon: '↕️',
            label: 'Scrolling',
            ariaLabel: 'Scrolling settings',
            title: 'How dragging scrolls, and touch gestures',
            extraClass: '',
            handler: toggleScrollFullPanel
        },
        {
            key: 'mouse',
            icon: '🖱️',
            label: mouseMode === 'none' ? 'Mouse' : 'Mouse ON',
            ariaLabel: mouseMode === 'none' ? 'Enable mouse mode for terminal apps (vim, tmux)' : 'Disable mouse mode — enables text selection',
            title: mouseMode === 'none' ? 'Mouse OFF — tap to enable for vim/tmux' : 'Mouse ON — tap to disable, enables selection',
            extraClass: mouseMode === 'any' ? mouseModeActive : '',
            handler: ()=>{
                track({
                    name: "toolbar_button_click",
                    category: "user_action",
                    sessionId,
                    component: "TerminalOutput",
                    labels: {
                        button: "mouse",
                        state: mouseMode === 'none' ? 'on' : 'off'
                    }
                });
                toggleMouseMode();
            }
        }
    ];
    const secondaryActions = readOnly ? allSecondaryActions.filter((a)=>a.key !== 'paste') : allSecondaryActions;
    const isConnectingState = terminalState === "CONNECTING" || terminalState === "LOADING";
    // Rendered in both the desktop toolbar and the mobile overflow row; the hidden file <input>s stay in toolbarActions. The overflow row stays open on click so upload status on the Gallery label remains visible.
    const renderUploadButtons = ()=>readOnly ? null : /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
            children: [
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                    className: toolbarButton,
                    onClick: ()=>{
                        track({
                            name: "toolbar_button_click",
                            category: "user_action",
                            sessionId,
                            component: "TerminalOutput",
                            labels: {
                                button: "gallery"
                            }
                        });
                        handleGalleryButtonClick();
                    },
                    disabled: uploadingCount > 0,
                    title: "Attach image(s) from gallery — multi-select supported",
                    "aria-label": uploadingCount > 0 ? `Uploading ${uploadingCount} file(s)...` : "Attach images from gallery",
                    children: uploadSuccess ? `✅ ${uploadSuccess}` : uploadError ? `⚠️ ${uploadError}` : uploadingCount > 0 ? `⏳ ${uploadingCount}…` : "🖼️ Gallery"
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                    className: toolbarButton,
                    onClick: ()=>{
                        track({
                            name: "toolbar_button_click",
                            category: "user_action",
                            sessionId,
                            component: "TerminalOutput",
                            labels: {
                                button: "files"
                            }
                        });
                        handleFilesButtonClick();
                    },
                    disabled: uploadingCount > 0,
                    title: "Attach any file(s) — multi-select, all types accepted",
                    "aria-label": uploadingCount > 0 ? `Uploading ${uploadingCount} file(s)...` : "Attach files",
                    children: uploadingCount > 0 ? `⏳ ${uploadingCount}…` : "📁 Files"
                }),
                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                    className: `${toolbarButton} ${mobileOnlyUpload}`,
                    onClick: ()=>{
                        track({
                            name: "toolbar_button_click",
                            category: "user_action",
                            sessionId,
                            component: "TerminalOutput",
                            labels: {
                                button: "camera"
                            }
                        });
                        handleCameraButtonClick();
                    },
                    disabled: uploadingCount > 0,
                    title: "Take a photo — opens camera directly",
                    "aria-label": uploadingCount > 0 ? `Uploading ${uploadingCount} file(s)...` : "Take photo with camera",
                    children: uploadingCount > 0 ? `⏳ ${uploadingCount}…` : "📷"
                })
            ]
        });
    return /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
        className: container,
        children: [
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: toolbar,
                ref: toolbarRef,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: TerminalOutput_css_status,
                        children: [
                            isExternal && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: externalLabel,
                                title: "External session via ssq-mux",
                                children: "\uD83D\uDD17 External"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: `${statusIndicator} ${isConnected ? connected : isWaitingForStableSize || isConnectingState ? stabilizing : disconnected}`
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: statusText,
                                children: isConnected ? "Connected" : isWaitingForStableSize ? "Initializing..." : isConnectingState ? "Connecting..." : "Disconnected"
                            }),
                            !isConnected && connectionAttempts > 0 && connectionAttempts < 5 && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                                className: statusText,
                                children: [
                                    " ",
                                    "• Reconnecting (attempt ",
                                    connectionAttempts,
                                    "/5)..."
                                ]
                            }),
                            !isConnected && connectionAttempts >= 5 && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: errorText,
                                children: " • Terminal unavailable"
                            }),
                            isHardFailed && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                className: errorText,
                                children: " • Terminal unavailable"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ConnectionCountIndicator, {
                                count: connectionCount,
                                sizeMismatch: hasResizeSizeMismatch,
                                pulse: connectionPulse
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: actions,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: toolbarToggle,
                                onClick: ()=>setToolbarExpanded((v)=>!v),
                                "aria-label": "Toggle toolbar",
                                "aria-expanded": toolbarExpanded,
                                "data-testid": "toolbar-toggle",
                                children: toolbarExpanded ? '✕' : '⋯'
                            }),
                            !readOnly && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: `${toolbarButton} ${mobileKeyboardToggle}`,
                                onClick: toggleMobileKeyboard,
                                "aria-label": isKeyboardVisible ? "Hide mobile keyboard" : "Show mobile keyboard",
                                "aria-expanded": isKeyboardVisible,
                                title: isKeyboardVisible ? "Hide mobile keyboard" : "Show mobile keyboard",
                                children: "⌨️"
                            }),
                            showReconnectButton && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: toolbarButton,
                                onClick: handleManualReconnect,
                                title: "Reconnect to terminal",
                                "aria-label": "Reconnect to terminal",
                                children: "\uD83D\uDD04 Reconnect"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                                className: toolbarButton,
                                onClick: ()=>{
                                    track({
                                        name: "toolbar_button_click",
                                        category: "user_action",
                                        sessionId,
                                        component: "TerminalOutput",
                                        labels: {
                                            button: "resize"
                                        }
                                    });
                                    handleManualResize();
                                },
                                "aria-label": "Redraw terminal (fixes a blank screen)",
                                title: "Redraw terminal (fixes a blank screen)",
                                children: [
                                    "↔️",
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                        className: toolbarButtonLabel,
                                        children: " Redraw"
                                    })
                                ]
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ScrollModeChip, {
                                effectiveTarget: effectiveScroll.target,
                                gestureScrollEnabled: gestureScrollEnabled,
                                onClick: openScrollPicker,
                                visibleRows: terminalRows,
                                highlighted: misrouteCue.highlighted,
                                announcement: misrouteCue.announcement,
                                suppressAnnouncements: scrollPanel !== null,
                                getTouchSurface: getTouchSurface,
                                buttonRef: scrollChipRef
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                role: "status",
                                "aria-live": "polite",
                                className: LiveRegion_css/* srOnly */.s,
                                "data-testid": "scroll-mode-announcer",
                                children: scrollAnnouncement
                            }),
                            toolbarExpanded && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                className: toolbarActions,
                                "data-testid": "toolbar-actions",
                                children: [
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                        className: secondaryGroup,
                                        "data-testid": "toolbar-secondary",
                                        children: secondaryActions.map((action)=>// action.handler (defined in secondaryActions above) always calls track() itself
                                            // analytics-exempt
                                            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                                                className: `${toolbarButton}${action.extraClass ? ` ${action.extraClass}` : ''}`,
                                                onClick: action.handler,
                                                "aria-label": action.ariaLabel,
                                                title: action.title,
                                                children: [
                                                    action.icon,
                                                    " ",
                                                    action.label
                                                ]
                                            }, action.key))
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                        ref: cameraInputRef,
                                        type: "file",
                                        accept: "image/*",
                                        capture: "environment",
                                        style: {
                                            display: "none"
                                        },
                                        onChange: handleFilesUpload,
                                        "aria-hidden": "true"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                        ref: galleryInputRef,
                                        type: "file",
                                        accept: "image/*",
                                        multiple: true,
                                        style: {
                                            display: "none"
                                        },
                                        onChange: handleFilesUpload,
                                        "aria-hidden": "true"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("input", {
                                        ref: filesInputRef,
                                        type: "file",
                                        accept: "*/*",
                                        multiple: true,
                                        style: {
                                            display: "none"
                                        },
                                        onChange: handleFilesUpload,
                                        "aria-hidden": "true"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                                        className: desktopOnlyUploads,
                                        children: renderUploadButtons()
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                        className: toolbarButton,
                                        onClick: ()=>{
                                            track({
                                                name: "toolbar_button_click",
                                                category: "user_action",
                                                sessionId,
                                                component: "TerminalOutput",
                                                labels: {
                                                    button: "output-filter",
                                                    state: filterOpen ? "closed" : "open"
                                                }
                                            });
                                            setFilterOpen((open)=>!open);
                                        },
                                        "aria-label": filterOpen ? "Close output filter" : "Filter terminal output",
                                        "aria-pressed": filterOpen,
                                        "data-testid": "toolbar-filter-toggle",
                                        title: "Filter terminal output (display only)",
                                        children: "\uD83D\uDD0E"
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                                        ref: devToggleRef,
                                        className: `${toolbarButton} ${devOnly}`,
                                        onClick: handleDevGroupToggle,
                                        "aria-label": devGroupOpen ? "Collapse developer tools" : "Expand developer tools",
                                        "aria-expanded": devGroupOpen,
                                        "aria-controls": "toolbar-dev-group-inner",
                                        "data-testid": "toolbar-dev-toggle",
                                        title: "Developer tools: Debug, Log Stream, Record, Raw",
                                        children: [
                                            "⚙",
                                            devGroupOpen ? ' ▴' : ' ▾'
                                        ]
                                    }),
                                    devGroupOpen && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                        className: devGroupPanel,
                                        "data-testid": "toolbar-dev-group",
                                        children: /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                                            id: "toolbar-dev-group-inner",
                                            role: "group",
                                            "aria-label": "Developer tools",
                                            className: devGroup,
                                            children: [
                                                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                                                    className: `${toolbarButton} ${devOnly} ${debugMode ? debugActive : ''}`,
                                                    onClick: ()=>{
                                                        track({
                                                            name: "toolbar_button_click",
                                                            category: "user_action",
                                                            sessionId,
                                                            component: "TerminalOutput",
                                                            labels: {
                                                                button: "debug",
                                                                state: debugMode ? "off" : "on"
                                                            }
                                                        });
                                                        handleToggleDebug();
                                                    },
                                                    title: debugMode ? "Disable debug logging" : "Enable debug logging",
                                                    "aria-label": debugMode ? "Disable debug mode" : "Enable debug mode",
                                                    style: debugMode ? {
                                                        backgroundColor: '#2a4',
                                                        color: 'white',
                                                        fontWeight: 'bold'
                                                    } : {},
                                                    children: [
                                                        "\uD83D\uDEE0️ ",
                                                        debugMode ? 'Debug ON' : 'Debug'
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                                                    className: `${toolbarButton} ${devOnly} ${logStreamEnabled ? debugActive : ''}`,
                                                    onClick: ()=>{
                                                        track({
                                                            name: "toolbar_button_click",
                                                            category: "user_action",
                                                            sessionId,
                                                            component: "TerminalOutput",
                                                            labels: {
                                                                button: "log-stream",
                                                                state: logStreamEnabled ? "off" : "on"
                                                            }
                                                        });
                                                        handleToggleLogStream();
                                                    },
                                                    title: logStreamEnabled ? "Stop forwarding verbose per-frame debug traces to server (info/warn/error always stream)" : "Also forward verbose per-frame debug traces to server (info/warn/error always stream automatically)",
                                                    "aria-label": logStreamEnabled ? "Disable verbose debug trace streaming" : "Enable verbose debug trace streaming",
                                                    style: logStreamEnabled ? {
                                                        backgroundColor: '#2a4',
                                                        color: 'white',
                                                        fontWeight: 'bold'
                                                    } : {},
                                                    children: [
                                                        "\uD83D\uDCE1 ",
                                                        logStreamEnabled ? 'Debug Traces ON' : 'Debug Traces'
                                                    ]
                                                }),
                                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                                    className: `${toolbarButton} ${devOnly}`,
                                                    onClick: ()=>{
                                                        track({
                                                            name: "toolbar_button_click",
                                                            category: "user_action",
                                                            sessionId,
                                                            component: "TerminalOutput",
                                                            labels: {
                                                                button: "record",
                                                                state: isRecording ? "off" : "on"
                                                            }
                                                        });
                                                        if (isRecording) {
                                                            stopRecording();
                                                            setIsRecording(false);
                                                        } else {
                                                            startRecording();
                                                            setIsRecording(true);
                                                        }
                                                    },
                                                    title: isRecording ? "Stop recording" : "Start recording terminal output",
                                                    "aria-label": isRecording ? "Stop recording terminal output" : "Start recording terminal output",
                                                    style: isRecording ? {
                                                        backgroundColor: '#ff4444',
                                                        color: 'white'
                                                    } : {},
                                                    children: isRecording ? '⏹️ Stop Rec' : '⏺️ Record'
                                                }),
                                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                                    className: `${toolbarButton} ${devOnly}`,
                                                    onClick: ()=>{
                                                        track({
                                                            name: "toolbar_button_click",
                                                            category: "user_action",
                                                            sessionId,
                                                            component: "TerminalOutput",
                                                            labels: {
                                                                button: "handedness",
                                                                state: leftHanded ? "right" : "left"
                                                            }
                                                        });
                                                        toggleHandedness();
                                                    },
                                                    "aria-label": leftHanded ? "Switch to right-handed layout" : "Switch to left-handed layout",
                                                    "aria-pressed": leftHanded,
                                                    title: "Toggle left/right-handed mobile layout",
                                                    children: leftHanded ? "✋ Left" : "🤚 Right"
                                                })
                                            ]
                                        })
                                    }),
                                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                        className: `${toolbarButton} ${mobileMoreButton} ${mobileOverflowOpen ? mobileMoreActive : ''}`,
                                        onClick: ()=>setMobileOverflowOpen((o)=>!o),
                                        "aria-label": mobileOverflowOpen ? 'Close more tools' : 'More tools',
                                        "aria-expanded": mobileOverflowOpen,
                                        "data-testid": "toolbar-more-button",
                                        children: mobileOverflowOpen ? '✕ Less' : 'More ▾'
                                    })
                                ]
                            })
                        ]
                    })
                ]
            }),
            mobileOverflowOpen && toolbarExpanded && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: mobileOverflowRow,
                "data-testid": "toolbar-overflow-row",
                ref: overflowRowRef,
                children: [
                    secondaryActions.map((action)=>// action.handler (defined in secondaryActions above) always calls track() itself
                        // analytics-exempt
                        /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("button", {
                            className: `${toolbarButton}${action.extraClass ? ` ${action.extraClass}` : ''}`,
                            onClick: ()=>{
                                action.handler();
                                setMobileOverflowOpen(false);
                            },
                            "aria-label": action.ariaLabel,
                            title: action.title,
                            children: [
                                action.icon,
                                " ",
                                action.label
                            ]
                        }, action.key)),
                    renderUploadButtons()
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ScrollHint, {
                visible: scrollHint.visible,
                route: scrollHint.route,
                onDismiss: scrollHint.dismiss
            }),
            scrollPanel && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ScrollingPanel, {
                variant: scrollPanel,
                override: scrollOverride,
                onOverrideChange: handleScrollOverrideChange,
                effectiveTarget: effectiveScroll.target,
                gestureScrollEnabled: gestureScrollEnabled,
                onGestureScrollChange: scrollSettings.setGestureScroll,
                onClose: closeScrollPanel,
                onOpenFull: openScrollFullPanel,
                openerRef: scrollChipRef,
                renderAsOverlay: panelAsOverlay,
                maxHeight: panelMaxHeight
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: terminal,
                ref: terminalContainerRef,
                ...dropHandlers,
                "data-testid": "terminal-drop-zone",
                children: [
                    isDragOver && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: dropZoneOverlay,
                        "aria-hidden": "true",
                        "data-testid": "drop-zone-overlay",
                        children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                            className: dropZoneLabel,
                            children: "Drop to attach"
                        })
                    }),
                    filterOpen && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(TerminalOutputFilter, {
                        getLines: getFilterLines,
                        onClose: ()=>setFilterOpen(false)
                    }),
                    showReconnectBanner && !isHardFailed && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: reconnectingBanner,
                        role: "status",
                        "aria-live": "polite",
                        "aria-label": "Reconnecting",
                        children: "Reconnecting terminal…"
                    }),
                    showReconnectBanner && isHardFailed && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: hardFailedBanner,
                        role: "alert",
                        children: (0,backoff/* isWorktreeMissingError */.GN)(error) ? /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                            children: [
                                "This session's working directory no longer exists (its git worktree may have been deleted).",
                                " ",
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                    onClick: handleRetryNow,
                                    disabled: isRetryingSession,
                                    children: isRetryingSession ? "Retrying…" : "Retry now"
                                })
                            ]
                        }) : /*#__PURE__*/ (0,react_jsx_runtime.jsxs)(react_jsx_runtime.Fragment, {
                            children: [
                                "Connection lost — ",
                                /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                    onClick: reconnectViaHook,
                                    children: "Retry"
                                })
                            ]
                        })
                    }),
                    isVisible !== false && isLoadingInitialContent && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: loadingOverlay,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                className: loadingSpinner
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                className: loadingText,
                                children: isWaitingForStableSize ? "Initializing terminal..." : !isConnected && !isInitialScrollbackDoneRef.current ? "Starting session..." : "Loading terminal content..."
                            })
                        ]
                    }),
                    !isLoadingInitialContent && connectionAttempts >= 5 && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: unavailableOverlay,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                className: unavailableIcon,
                                children: "⚠"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                className: unavailableText,
                                children: "Terminal unavailable"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                                className: unavailableSubtext,
                                children: "Could not connect to terminal session"
                            })
                        ]
                    }),
                    terminalState === 'RESIZING' && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: resizingOverlay,
                        role: "status",
                        "aria-label": "Terminal resizing",
                        children: /*#__PURE__*/ (0,react_jsx_runtime.jsx)("span", {
                            className: resizingSpinner
                        })
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ScrollLoadingPill, {
                        visible: pillState.visible,
                        stalled: pillState.stalled,
                        program: pillState.program,
                        onCancel: handleCancelScrollLoading
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)(ScrollSourceIndicator, {
                        program: appScrollbackState.program,
                        visible: appScrollbackState.active
                    }),
                    atTopVisible && /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        className: noMoreHistoryLine,
                        role: "status",
                        "aria-live": "polite",
                        "data-testid": "no-more-app-history",
                        children: "No more history available"
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("div", {
                        ref: dockRef,
                        className: TerminalPool_css/* dockAnchor */.ZL
                    }),
                    jumpTerminal?.buffer?.active && /*#__PURE__*/ (0,react_jsx_runtime.jsx)(JumpToLatestMount, {
                        terminal: jumpTerminal,
                        route: effectiveScroll.target,
                        netPagesUp: netPagesUp,
                        connectionEpoch: connectionEpoch,
                        sendData: deliverTerminalData,
                        isInputBusy: isInputChunking,
                        gestureActive: gestureActive,
                        getContainer: getTouchSurface
                    })
                ]
            }),
            /*#__PURE__*/ (0,react_jsx_runtime.jsx)(InputDropBadge, {
                count: dropEpisode.count,
                episodeSeq: dropEpisode.seq
            }),
            blockedToast && typeof document !== "undefined" && /*#__PURE__*/ (0,react_dom.createPortal)(/*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: TerminalOutput_css_blockedToast,
                role: "status",
                "aria-live": "polite",
                "data-testid": "scroll-blocked-toast",
                "data-blocked-reason": events_pb/* ScrollBlockedReason */.ec[blockedToast.reason],
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("span", {
                        children: [
                            "⚠ ",
                            blockedToastCopy(blockedToast.reason, blockedToast.program)
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                        type: "button",
                        className: blockedToastButton,
                        onClick: dismissBlockedToast,
                        "data-testid": "scroll-blocked-toast-dismiss",
                        children: "Got it"
                    })
                ]
            }, blockedToast.seq), document.body),
            isKeyboardVisible && !readOnly && /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                className: mobileKeyboard,
                children: [
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: mobileKeyRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\x1b');
                                },
                                "aria-label": "Escape",
                                "data-testid": "mobile-key",
                                children: "Esc"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('/');
                                },
                                "aria-label": "Forward slash",
                                "data-testid": "mobile-key",
                                children: "/"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('-');
                                },
                                "aria-label": "Hyphen",
                                "data-testid": "mobile-key",
                                children: "-"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\x1b[H');
                                },
                                "aria-label": "Home",
                                "data-testid": "mobile-key",
                                children: "Home"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\x1b[A');
                                },
                                "aria-label": "Up arrow",
                                "data-testid": "mobile-key",
                                children: "↑"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\x1b[F');
                                },
                                "aria-label": "End",
                                "data-testid": "mobile-key",
                                children: "End"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendToolbarPageKey("up");
                                },
                                "aria-label": "Page up",
                                "data-testid": "mobile-key",
                                children: "PgUp"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: mobileKeyRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\t');
                                },
                                "aria-label": "Tab",
                                "data-testid": "mobile-key",
                                children: "Tab"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: `${mobileKey} ${shiftActive ? mobileKeyActive : ''}`,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setShiftActive((p)=>!p);
                                    setCtrlActive(false);
                                    setAltActive(false);
                                },
                                "aria-label": shiftActive ? 'Shift active — press next key' : 'Shift modifier',
                                "aria-pressed": shiftActive,
                                "data-testid": "mobile-key",
                                children: "Shift"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: `${mobileKey} ${ctrlActive ? mobileKeyActive : ''}`,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive((p)=>!p);
                                    setAltActive(false);
                                    setShiftActive(false);
                                },
                                "aria-label": ctrlActive ? 'Ctrl active — press next key' : 'Control modifier',
                                "aria-pressed": ctrlActive,
                                "data-testid": "mobile-key",
                                children: "Ctrl"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: `${mobileKey} ${altActive ? mobileKeyActive : ''}`,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setAltActive((p)=>!p);
                                    setCtrlActive(false);
                                    setShiftActive(false);
                                },
                                "aria-label": altActive ? 'Alt active — press next key' : 'Alt modifier',
                                "aria-pressed": altActive,
                                "data-testid": "mobile-key",
                                children: "Alt"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\x1b[D');
                                },
                                "aria-label": "Left arrow",
                                "data-testid": "mobile-key",
                                children: "←"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\x1b[B');
                                },
                                "aria-label": "Down arrow",
                                "data-testid": "mobile-key",
                                children: "↓"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendKey('\x1b[C');
                                },
                                "aria-label": "Right arrow",
                                "data-testid": "mobile-key",
                                children: "→"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    sendToolbarPageKey("down");
                                },
                                "aria-label": "Page down",
                                "data-testid": "mobile-key",
                                children: "PgDn"
                            })
                        ]
                    }),
                    /*#__PURE__*/ (0,react_jsx_runtime.jsxs)("div", {
                        className: mobileKeyRow,
                        children: [
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: `${mobileKey} ${mobileKeyCtrlC}`,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive(false);
                                    setAltActive(false);
                                    handleTerminalData('\x03');
                                },
                                "aria-label": "Ctrl+C (interrupt)",
                                title: "Interrupt (Ctrl+C)",
                                "data-testid": "mobile-key",
                                children: "^C"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive(false);
                                    setAltActive(false);
                                    handleTerminalData('\x1a');
                                },
                                "aria-label": "Ctrl+Z (suspend)",
                                title: "Suspend (Ctrl+Z)",
                                "data-testid": "mobile-key",
                                children: "^Z"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive(false);
                                    setAltActive(false);
                                    handleTerminalData('\x0c');
                                },
                                "aria-label": "Ctrl+L (clear screen)",
                                title: "Clear screen (Ctrl+L)",
                                "data-testid": "mobile-key",
                                children: "^L"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive(false);
                                    setAltActive(false);
                                    handleTerminalData('\x12');
                                },
                                "aria-label": "Ctrl+R (reverse search)",
                                title: "Reverse search (Ctrl+R)",
                                "data-testid": "mobile-key",
                                children: "^R"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive(false);
                                    setAltActive(false);
                                    handleTerminalData('\x17');
                                },
                                "aria-label": "Ctrl+W (delete word)",
                                title: "Delete word (Ctrl+W)",
                                "data-testid": "mobile-key",
                                children: "^W"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive(false);
                                    setAltActive(false);
                                    handleTerminalData('\x15');
                                },
                                "aria-label": "Ctrl+U (delete line)",
                                title: "Delete line (Ctrl+U)",
                                "data-testid": "mobile-key",
                                children: "^U"
                            }),
                            /*#__PURE__*/ (0,react_jsx_runtime.jsx)("button", {
                                className: mobileKey,
                                onPointerDown: (e)=>{
                                    e.preventDefault();
                                    setCtrlActive(false);
                                    setAltActive(false);
                                    handleTerminalData('\x04');
                                },
                                "aria-label": "Ctrl+D (EOF)",
                                title: "EOF / logout (Ctrl+D)",
                                "data-testid": "mobile-key",
                                children: "^D"
                            })
                        ]
                    })
                ]
            })
        ]
    });
}


/***/ }),

/***/ 20606:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   L: () => (/* binding */ LiveRegion)
/* harmony export */ });
/* unused harmony export useLiveRegion */
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(79147);
/* harmony import */ var react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_1___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_1__);
/* harmony import */ var _LiveRegion_css__WEBPACK_IMPORTED_MODULE_2__ = __webpack_require__(40855);
/* __next_internal_client_entry_do_not_use__ LiveRegion,useLiveRegion auto */ 


function LiveRegion({ message, politeness = "polite", role = "status" }) {
    const [currentMessage, setCurrentMessage] = (0,react__WEBPACK_IMPORTED_MODULE_1__.useState)(message);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_1__.useEffect)(()=>{
        if (message) {
            setCurrentMessage(message);
        }
    }, [
        message
    ]);
    return /*#__PURE__*/ (0,react_jsx_runtime__WEBPACK_IMPORTED_MODULE_0__.jsx)("div", {
        role: role,
        "aria-live": politeness,
        "aria-atomic": "true",
        className: _LiveRegion_css__WEBPACK_IMPORTED_MODULE_2__/* .srOnly */ .s,
        children: currentMessage
    });
}
// Hook to use live region announcements
function useLiveRegion() {
    const [message, setMessage] = useState("");
    const announce = (newMessage)=>{
        setMessage(newMessage);
        // Clear message after announcement
        setTimeout(()=>setMessage(""), 1000);
    };
    return {
        message,
        announce
    };
}


/***/ }),

/***/ 20670:
/***/ ((module) => {

"use strict";


const BINARY_TYPES = ['nodebuffer', 'arraybuffer', 'fragments'];
const hasBlob = typeof Blob !== 'undefined';

if (hasBlob) BINARY_TYPES.push('blob');

module.exports = {
  BINARY_TYPES,
  EMPTY_BUFFER: Buffer.alloc(0),
  GUID: '258EAFA5-E914-47DA-95CA-C5AB0DC85B11',
  hasBlob,
  kForOnEventAttribute: Symbol('kIsForOnEventAttribute'),
  kListener: Symbol('kListener'),
  kStatusCode: Symbol('status-code'),
  kWebSocket: Symbol('websocket'),
  NOOP: () => {}
};


/***/ }),

/***/ 21579:
/***/ (() => {

/* (ignored) */

/***/ }),

/***/ 21817:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "._2yato10 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz30);\n  padding: var(--_19ez2rz30);\n  background: var(--_19ez2rz7);\n  color: var(--_19ez2rz0);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3b);\n}\n._2yato10[data-overlay=\"true\"] {\n  position: absolute;\n  left: 0;\n  right: 0;\n  z-index: 25;\n  overflow-y: auto;\n}\n._2yato11 {\n  border: none;\n  margin: 0;\n  padding: 0;\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2z);\n}\n._2yato12 {\n  font-weight: var(--_19ez2rz3g);\n  padding: 0;\n  margin-bottom: var(--_19ez2rz2z);\n}\n._2yato13 {\n  display: flex;\n  align-items: flex-start;\n  gap: var(--_19ez2rz30);\n  min-height: 44px;\n  min-width: 44px;\n  padding: var(--_19ez2rz2z);\n  cursor: pointer;\n}\n._2yato14 {\n  flex-shrink: 0;\n  width: 24px;\n  height: 24px;\n  margin: 10px 0 0 0;\n}\n._2yato14:focus-visible {\n  outline: 2px solid var(--_19ez2rzm);\n  outline-offset: 2px;\n}\n._2yato15 {\n  display: flex;\n  flex-direction: column;\n  gap: var(--_19ez2rz2y);\n}\n._2yato16 {\n  font-weight: var(--_19ez2rz3g);\n}\n._2yato17 {\n  color: var(--_19ez2rz1);\n}\n._2yato18 {\n  display: flex;\n  align-items: flex-start;\n  gap: var(--_19ez2rz30);\n  min-height: 44px;\n  padding: var(--_19ez2rz2z);\n  cursor: pointer;\n}\n._2yato19 {\n  margin: 0;\n  color: var(--_19ez2rz1);\n}\n._2yato1a {\n  min-height: 44px;\n  min-width: 44px;\n  padding: var(--_19ez2rz2y) var(--_19ez2rz30);\n  background: transparent;\n  color: var(--_19ez2rz0);\n  border: 1px solid var(--_19ez2rzh);\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3b);\n  cursor: pointer;\n}\n._2yato1a:focus-visible {\n  outline: 2px solid var(--_19ez2rzm);\n  outline-offset: 2px;\n}\n._2yato1b {\n  display: inline-flex;\n  align-items: center;\n  justify-content: center;\n  min-height: 44px;\n  min-width: 44px;\n  padding: var(--_19ez2rz2y) var(--_19ez2rz30);\n  background: var(--_19ez2rz7);\n  color: var(--_19ez2rz0);\n  border: 1px solid var(--_19ez2rzh);\n  border-radius: var(--_19ez2rz39);\n  font-size: var(--_19ez2rz3b);\n  cursor: pointer;\n  white-space: nowrap;\n  flex-shrink: 0;\n}\n._2yato1b:focus-visible {\n  outline: 2px solid var(--_19ez2rzm);\n  outline-offset: 2px;\n}\n._2yato1b[data-highlighted=\"true\"] {\n  border-width: 3px;\n  font-weight: var(--_19ez2rz3g);\n}\n._2yato1c {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz30);\n  padding: var(--_19ez2rz2z);\n  background: var(--_19ez2rz7);\n  color: var(--_19ez2rz0);\n  border: 1px solid var(--_19ez2rzh);\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3b);\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 25270:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";


const { tokenChars } = __webpack_require__(82524);

/**
 * Adds an offer to the map of extension offers or a parameter to the map of
 * parameters.
 *
 * @param {Object} dest The map of extension offers or parameters
 * @param {String} name The extension or parameter name
 * @param {(Object|Boolean|String)} elem The extension parameters or the
 *     parameter value
 * @private
 */
function push(dest, name, elem) {
  if (dest[name] === undefined) dest[name] = [elem];
  else dest[name].push(elem);
}

/**
 * Parses the `Sec-WebSocket-Extensions` header into an object.
 *
 * @param {String} header The field value of the header
 * @return {Object} The parsed object
 * @public
 */
function parse(header) {
  const offers = Object.create(null);
  let params = Object.create(null);
  let mustUnescape = false;
  let isEscaping = false;
  let inQuotes = false;
  let extensionName;
  let paramName;
  let start = -1;
  let code = -1;
  let end = -1;
  let i = 0;

  for (; i < header.length; i++) {
    code = header.charCodeAt(i);

    if (extensionName === undefined) {
      if (end === -1 && tokenChars[code] === 1) {
        if (start === -1) start = i;
      } else if (
        i !== 0 &&
        (code === 0x20 /* ' ' */ || code === 0x09) /* '\t' */
      ) {
        if (end === -1 && start !== -1) end = i;
      } else if (code === 0x3b /* ';' */ || code === 0x2c /* ',' */) {
        if (start === -1) {
          throw new SyntaxError(`Unexpected character at index ${i}`);
        }

        if (end === -1) end = i;
        const name = header.slice(start, end);
        if (code === 0x2c) {
          push(offers, name, params);
          params = Object.create(null);
        } else {
          extensionName = name;
        }

        start = end = -1;
      } else {
        throw new SyntaxError(`Unexpected character at index ${i}`);
      }
    } else if (paramName === undefined) {
      if (end === -1 && tokenChars[code] === 1) {
        if (start === -1) start = i;
      } else if (code === 0x20 || code === 0x09) {
        if (end === -1 && start !== -1) end = i;
      } else if (code === 0x3b || code === 0x2c) {
        if (start === -1) {
          throw new SyntaxError(`Unexpected character at index ${i}`);
        }

        if (end === -1) end = i;
        push(params, header.slice(start, end), true);
        if (code === 0x2c) {
          push(offers, extensionName, params);
          params = Object.create(null);
          extensionName = undefined;
        }

        start = end = -1;
      } else if (code === 0x3d /* '=' */ && start !== -1 && end === -1) {
        paramName = header.slice(start, i);
        start = end = -1;
      } else {
        throw new SyntaxError(`Unexpected character at index ${i}`);
      }
    } else {
      //
      // The value of a quoted-string after unescaping must conform to the
      // token ABNF, so only token characters are valid.
      // Ref: https://tools.ietf.org/html/rfc6455#section-9.1
      //
      if (isEscaping) {
        if (tokenChars[code] !== 1) {
          throw new SyntaxError(`Unexpected character at index ${i}`);
        }
        if (start === -1) start = i;
        else if (!mustUnescape) mustUnescape = true;
        isEscaping = false;
      } else if (inQuotes) {
        if (tokenChars[code] === 1) {
          if (start === -1) start = i;
        } else if (code === 0x22 /* '"' */ && start !== -1) {
          inQuotes = false;
          end = i;
        } else if (code === 0x5c /* '\' */) {
          isEscaping = true;
        } else {
          throw new SyntaxError(`Unexpected character at index ${i}`);
        }
      } else if (code === 0x22 && header.charCodeAt(i - 1) === 0x3d) {
        inQuotes = true;
      } else if (end === -1 && tokenChars[code] === 1) {
        if (start === -1) start = i;
      } else if (start !== -1 && (code === 0x20 || code === 0x09)) {
        if (end === -1) end = i;
      } else if (code === 0x3b || code === 0x2c) {
        if (start === -1) {
          throw new SyntaxError(`Unexpected character at index ${i}`);
        }

        if (end === -1) end = i;
        let value = header.slice(start, end);
        if (mustUnescape) {
          value = value.replace(/\\/g, '');
          mustUnescape = false;
        }
        push(params, paramName, value);
        if (code === 0x2c) {
          push(offers, extensionName, params);
          params = Object.create(null);
          extensionName = undefined;
        }

        paramName = undefined;
        start = end = -1;
      } else {
        throw new SyntaxError(`Unexpected character at index ${i}`);
      }
    }
  }

  if (start === -1 || inQuotes || code === 0x20 || code === 0x09) {
    throw new SyntaxError('Unexpected end of input');
  }

  if (end === -1) end = i;
  const token = header.slice(start, end);
  if (extensionName === undefined) {
    push(offers, token, params);
  } else {
    if (paramName === undefined) {
      push(params, token, true);
    } else if (mustUnescape) {
      push(params, paramName, token.replace(/\\/g, ''));
    } else {
      push(params, paramName, token);
    }
    push(offers, extensionName, params);
  }

  return offers;
}

/**
 * Builds the `Sec-WebSocket-Extensions` header field value.
 *
 * @param {Object} extensions The map of extensions and parameters to format
 * @return {String} A string representing the given object
 * @public
 */
function format(extensions) {
  return Object.keys(extensions)
    .map((extension) => {
      let configurations = extensions[extension];
      if (!Array.isArray(configurations)) configurations = [configurations];
      return configurations
        .map((params) => {
          return [extension]
            .concat(
              Object.keys(params).map((k) => {
                let values = params[k];
                if (!Array.isArray(values)) values = [values];
                return values
                  .map((v) => (v === true ? k : `${k}=${v}`))
                  .join('; ');
              })
            )
            .join('; ');
        })
        .join(', ');
    })
    .join(', ');
}

module.exports = { format, parse };


/***/ }),

/***/ 26550:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   H$: () => (/* binding */ decideScrollTarget),
/* harmony export */   Md: () => (/* binding */ PAGE_FLING_CAP),
/* harmony export */   Mu: () => (/* binding */ PAGE_RATE_LIMIT_MS),
/* harmony export */   Nl: () => (/* binding */ PAGE_DOWN_BYTES),
/* harmony export */   Ql: () => (/* binding */ decideAndLogScrollTarget),
/* harmony export */   UU: () => (/* binding */ PAGE_UP_BYTES),
/* harmony export */   YA: () => (/* binding */ ROUTING_VERIFIED),
/* harmony export */   d4: () => (/* binding */ toolbarPageAction),
/* harmony export */   gU: () => (/* binding */ encodeWheel),
/* harmony export */   qC: () => (/* binding */ encodePageKeys),
/* harmony export */   yC: () => (/* binding */ PageAccumulator)
/* harmony export */ });
/* unused harmony exports TUI_SCROLL_POLICY, DEFAULT_ROUTING_POLICY, MAX_WHEEL_REPORTS_PER_FRAME, encodeWheelX10, tuiPageStepLines */
/**
 * Scroll routing for touch drags: decides where a scroll step is delivered
 * (xterm local scrollback vs. TUI via wheel reports or PgUp/PgDn) and encodes
 * the bytes. Pure and deterministic; clocks and loggers are injected.
 */ /** Flips to true once the Phase 0 spike confirms the default rows. */ const ROUTING_VERIFIED = false;
const TUI_SCROLL_POLICY = "pgkeys";
const TRACKING_ON = [
    "x10",
    "vt200",
    "drag",
    "any"
];
const DEFAULT_ROUTING_POLICY = {
    rules: [
        {
            when: {
                bufferType: "alternate"
            },
            target: "tui-pgkeys"
        },
        ...TRACKING_ON.map((mouseTrackingMode)=>({
                when: {
                    mouseTrackingMode
                },
                target: "tui-pgkeys"
            }))
    ],
    default: "xterm-local"
};
function ruleMatches(when, mode) {
    return (when.bufferType === undefined || when.bufferType === mode.bufferType) && (when.mouseTrackingMode === undefined || when.mouseTrackingMode === mode.mouseTrackingMode);
}
/** First matching row wins; `tui-wheel` is only reachable when tuiPolicy is 'wheel'. */ function decideScrollTarget(mode, policy = DEFAULT_ROUTING_POLICY, tuiPolicy = TUI_SCROLL_POLICY, override = "auto") {
    const tuiTarget = tuiPolicy === "wheel" ? "tui-wheel" : "tui-pgkeys";
    if (override === "local") return "xterm-local";
    if (override === "tui") return tuiTarget;
    const row = policy.rules.find((r)=>ruleMatches(r.when, mode));
    const target = row ? row.target : policy.default;
    return target === "xterm-local" ? target : tuiTarget;
}
/** decideScrollTarget plus a log entry carrying `unverified:true` while routing is unverified. */ function decideAndLogScrollTarget(mode, opts = {}) {
    const override = opts.override ?? "auto";
    const target = decideScrollTarget(mode, opts.policy, opts.tuiPolicy, override);
    const verified = opts.routingVerified ?? ROUTING_VERIFIED;
    const entry = {
        target,
        source: override === "auto" ? "auto" : "override",
        mode,
        ...verified ? {} : {
            unverified: true
        }
    };
    opts.log?.(entry);
    return target;
}
// ---- Encoders ----
const MAX_WHEEL_REPORTS_PER_FRAME = 3;
function clamp(v, lo, hi) {
    return Math.min(Math.max(v, lo), hi);
}
/**
 * SGR wheel reports. lineDelta < 0 = toward older output (wheel up, button 64),
 * > 0 = toward newer (button 65). Emits at most MAX_WHEEL_REPORTS_PER_FRAME reports.
 */ function encodeWheel(lineDelta, cell) {
    const count = Math.min(Math.abs(Math.trunc(lineDelta)), MAX_WHEEL_REPORTS_PER_FRAME);
    if (count === 0) return "";
    const col = clamp(Math.trunc(cell.col), 1, Math.max(1, cell.cols));
    const row = clamp(Math.trunc(cell.row), 1, Math.max(1, cell.rows));
    const button = lineDelta < 0 ? 64 : 65;
    return `\x1b[<${button};${col};${row}M`.repeat(count);
}
/** X10 fallback; retained for the spike but deliberately not selectable via decideScrollTarget. */ function encodeWheelX10(lineDelta, cell) {
    const count = Math.min(Math.abs(Math.trunc(lineDelta)), MAX_WHEEL_REPORTS_PER_FRAME);
    if (count === 0) return "";
    const col = clamp(Math.trunc(cell.col), 1, Math.max(1, cell.cols));
    const row = clamp(Math.trunc(cell.row), 1, Math.max(1, cell.rows));
    const button = lineDelta < 0 ? 64 : 65;
    // X10 encodes each value as a single byte offset by 32 (limited to 223 cells).
    const ch = (n)=>String.fromCharCode(clamp(n, 0, 223) + 32);
    return `\x1b[M${ch(button)}${ch(col)}${ch(row)}`.repeat(count);
}
const PAGE_UP_BYTES = "\x1b[5~";
const PAGE_DOWN_BYTES = "\x1b[6~";
/** direction 'older' = PgUp, 'newer' = PgDn (same bytes as the toolbar keys). */ function encodePageKeys(direction, count = 1) {
    return (direction === "older" ? PAGE_UP_BYTES : PAGE_DOWN_BYTES).repeat(Math.max(0, count));
}
// ---- Page accumulator ----
const PAGE_RATE_LIMIT_MS = 100;
const PAGE_FLING_CAP = 5;
/** Post-slop finger travel (lines) that emits one page key: half a page. */ function tuiPageStepLines(rows) {
    return Math.max(1, Math.floor((rows - 1) / 2));
}
/**
 * Accumulates line travel into page keys. Positive lines = newer (PgDn),
 * negative = older (PgUp). Emits at most one key per call (frame), carries the
 * remainder, rate-limits to one per rateLimitMs and caps keys per fling.
 */ class PageAccumulator {
    constructor(opts){
        this.carry = 0;
        this.emitted = 0;
        this.lastEmitAt = Number.NEGATIVE_INFINITY;
        this.rows = opts.rows;
        this.now = opts.now;
        this.rateLimitMs = opts.rateLimitMs ?? PAGE_RATE_LIMIT_MS;
        this.flingCap = opts.flingCap ?? PAGE_FLING_CAP;
    }
    setRows(rows) {
        this.rows = rows;
    }
    /** True once the per-fling cap is hit; callers cancel momentum. */ get capped() {
        return this.emitted >= this.flingCap;
    }
    /** Returns the page-key bytes for this frame ("" when none). */ push(lines) {
        this.carry += lines;
        const step = tuiPageStepLines(this.rows);
        if (Math.abs(this.carry) < step) return "";
        if (this.capped) {
            this.carry = 0;
            return "";
        }
        const t = this.now();
        if (t - this.lastEmitAt < this.rateLimitMs) return "";
        const older = this.carry < 0;
        this.carry += older ? step : -step;
        this.emitted += 1;
        this.lastEmitAt = t;
        return encodePageKeys(older ? "older" : "newer");
    }
    /** Reset at gesture start / mode flip. */ reset() {
        this.carry = 0;
        this.emitted = 0;
        this.lastEmitAt = Number.NEGATIVE_INFINITY;
    }
}
/**
 * Same route as the drag, so the toolbar is an equivalent single-pointer
 * alternative. A modifier always sends bytes; in `auto`, a local route at its
 * edge falls through to bytes (today's behaviour for a normal-buffer TUI the
 * table misclassifies as local); an explicit `local` override never falls through.
 */ function toolbarPageAction(input) {
    const { route, direction, modifiers, canScroll, override } = input;
    const bytes = direction === "up" ? PAGE_UP_BYTES : PAGE_DOWN_BYTES;
    const modified = modifiers.ctrl || modifiers.alt || modifiers.shift;
    const isTui = route !== "xterm-local";
    if (modified) return {
        type: "send-keys",
        bytes,
        countsTowardNetPages: false
    };
    if (isTui) return {
        type: "send-keys",
        bytes,
        countsTowardNetPages: true
    };
    if (!canScroll && override !== "local") return {
        type: "send-keys",
        bytes,
        countsTowardNetPages: false
    };
    return {
        type: "scroll-pages",
        pages: direction === "up" ? -1 : 1
    };
}


/***/ }),

/***/ 29695:
/***/ ((module) => {

"use strict";


const kDone = Symbol('kDone');
const kRun = Symbol('kRun');

/**
 * A very simple job queue with adjustable concurrency. Adapted from
 * https://github.com/STRML/async-limiter
 */
class Limiter {
  /**
   * Creates a new `Limiter`.
   *
   * @param {Number} [concurrency=Infinity] The maximum number of jobs allowed
   *     to run concurrently
   */
  constructor(concurrency) {
    this[kDone] = () => {
      this.pending--;
      this[kRun]();
    };
    this.concurrency = concurrency || Infinity;
    this.jobs = [];
    this.pending = 0;
  }

  /**
   * Adds a job to the queue.
   *
   * @param {Function} job The job to run
   * @public
   */
  add(job) {
    this.jobs.push(job);
    this[kRun]();
  }

  /**
   * Removes a job from the queue and runs it if possible.
   *
   * @private
   */
  [kRun]() {
    if (this.pending === this.concurrency) return;

    if (this.jobs.length) {
      const job = this.jobs.shift();

      this.pending++;
      job(this[kDone]);
    }
  }
}

module.exports = Limiter;


/***/ }),

/***/ 29953:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "@keyframes _2a92ce0 {\n  0%, 100% {\n    opacity: 1;\n  }\n  50% {\n    opacity: 0.5;\n  }\n}\n@keyframes _2a92ce1 {\n  0% {\n    transform: rotate(0deg);\n  }\n  100% {\n    transform: rotate(360deg);\n  }\n}\n@keyframes _2a92ce16 {\n  0% {\n    transform: rotate(0deg);\n  }\n  100% {\n    transform: rotate(360deg);\n  }\n}\n._2a92ce2 {\n  display: flex;\n  flex-direction: column;\n  height: 100%;\n  min-height: 0;\n  flex: 1 1;\n  background: var(--_19ez2rz1d);\n  color: var(--_19ez2rz1e);\n  font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace;\n}\n._2a92ce3 {\n  display: flex;\n  align-items: center;\n  justify-content: space-between;\n  padding: 0.75rem 1rem;\n  background: var(--_19ez2rz7);\n  border-bottom: 1px solid var(--_19ez2rze);\n  flex-shrink: 0;\n}\n._2a92ce4 {\n  display: flex;\n  align-items: center;\n  gap: 0.5rem;\n}\n._2a92ce5 {\n  width: 8px;\n  height: 8px;\n  border-radius: 50%;\n  animation: _2a92ce0 2s infinite;\n}\n._2a92ce6 {\n  background: var(--_19ez2rzu);\n  box-shadow: 0 0 8px var(--_19ez2rzu);\n}\n._2a92ce7 {\n  background: var(--_19ez2rz10);\n  box-shadow: 0 0 8px var(--_19ez2rz10);\n  animation-name: none;\n}\n._2a92ce8 {\n  background: var(--_19ez2rzx);\n  box-shadow: 0 0 8px var(--_19ez2rzx);\n  animation: _2a92ce0 0.8s infinite;\n}\n._2a92ce9 {\n  font-size: 0.875rem;\n  color: var(--_19ez2rz1);\n}\n._2a92cea {\n  font-size: 12px;\n  font-weight: 600;\n  color: var(--_19ez2rzm);\n  padding: 2px 8px;\n  background: var(--_19ez2rz17);\n  border-radius: 4px;\n  border: 1px solid var(--_19ez2rzf);\n}\n._2a92ceb {\n  font-size: 0.875rem;\n  color: var(--_19ez2rz10);\n  margin-left: 0.5rem;\n}\n._2a92cec {\n  display: flex;\n  gap: 0.5rem;\n}\n._2a92ced {\n  padding: 0.5rem 0.75rem;\n  background: var(--_19ez2rz8);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: 4px;\n  color: var(--_19ez2rz0);\n  font-size: 0.875rem;\n  cursor: pointer;\n  transition: background 0.2s, border-color 0.2s;\n}\n._2a92ced:hover {\n  background: var(--_19ez2rzb);\n  border-color: var(--_19ez2rzi);\n}\n._2a92ced:active {\n  background: var(--_19ez2rz7);\n}\n._2a92ceg {\n  display: contents;\n}\n._2a92cei {\n  padding: 0.5rem;\n  background: var(--_19ez2rz8);\n  border: 1px solid var(--_19ez2rze);\n  border-radius: 4px;\n  color: var(--_19ez2rz0);\n  font-size: 0.75rem;\n  cursor: pointer;\n  min-width: var(--min-touch-target, 44px);\n  min-height: var(--min-touch-target, 44px);\n  display: flex;\n  align-items: center;\n  justify-content: center;\n  flex-shrink: 0;\n}\n._2a92cei:hover {\n  background: var(--_19ez2rzb);\n  border-color: var(--_19ez2rzi);\n}\n._2a92cej {\n  display: flex;\n  gap: 0.5rem;\n  flex-wrap: wrap;\n}\n._2a92cel {\n  display: flex;\n  align-items: center;\n  gap: 0.5rem;\n  padding-left: 0.5rem;\n  border-left: 1px solid var(--_19ez2rze);\n}\n._2a92cem {\n  display: flex;\n  align-items: center;\n  gap: 0.5rem;\n}\n._2a92cen {\n  display: flex;\n  gap: 0.5rem;\n}\n._2a92ceo {\n  display: none;\n}\n._2a92cep {\n  background: var(--_19ez2rzb) !important;\n  border-color: var(--_19ez2rzi) !important;\n}\n._2a92ceq {\n  display: none;\n}\n:root[data-left-handed] ._2a92ceq {\n  flex-direction: row;\n}\n._2a92ceq > button {\n  flex-shrink: 0;\n  white-space: nowrap;\n}\n._2a92cer {\n  display: inline-flex;\n  align-items: center;\n}\n._2a92ces {\n  background: var(--_19ez2rz17) !important;\n  border-color: var(--_19ez2rzm) !important;\n  color: var(--_19ez2rzq) !important;\n}\n._2a92cet {\n  background: var(--_19ez2rz17) !important;\n  border-color: var(--_19ez2rzm) !important;\n  color: var(--_19ez2rzq) !important;\n}\n._2a92ceu {\n  padding: 1rem;\n  background: var(--_19ez2rz11);\n  border-bottom: 1px solid var(--_19ez2rz10);\n  color: var(--_19ez2rz10);\n  font-size: 0.875rem;\n}\n._2a92cev {\n  flex: 1 1;\n  min-height: 0;\n  margin: 0;\n  padding: 0;\n  background: var(--_19ez2rz1d);\n  color: var(--_19ez2rz1e);\n  overflow: hidden;\n  position: relative;\n  padding-left: var(--safe-area-left, 0px);\n  padding-right: var(--safe-area-right, 0px);\n}\n._2a92cew {\n  position: absolute;\n  inset: 0;\n  z-index: 20;\n  display: flex;\n  align-items: center;\n  justify-content: center;\n  background-color: rgba(0, 0, 0, 0.45);\n  pointer-events: none;\n}\n._2a92cex {\n  color: #fff;\n  font-size: 1rem;\n  font-weight: 600;\n  padding: 8px 16px;\n  border-radius: 6px;\n  border: 2px dashed rgba(255, 255, 255, 0.7);\n  background: rgba(0, 0, 0, 0.3);\n  -webkit-user-select: none;\n     -moz-user-select: none;\n          user-select: none;\n}\n._2a92cey {\n  position: absolute;\n  top: 0;\n  left: 0;\n  right: 0;\n  bottom: 0;\n  background: var(--_19ez2rza);\n  display: flex;\n  flex-direction: column;\n  align-items: center;\n  justify-content: center;\n  gap: 1rem;\n  z-index: 10;\n  -webkit-backdrop-filter: blur(2px);\n          backdrop-filter: blur(2px);\n}\n._2a92cez {\n  width: 48px;\n  height: 48px;\n  border: 4px solid var(--_19ez2rz1f);\n  border-top: 4px solid var(--_19ez2rzu);\n  border-radius: 50%;\n  animation: _2a92ce1 1s linear infinite;\n}\n._2a92ce10 {\n  font-size: 0.875rem;\n  color: var(--_19ez2rz1e);\n  font-weight: 500;\n}\n._2a92ce11 {\n  position: absolute;\n  top: 0;\n  left: 0;\n  right: 0;\n  bottom: 0;\n  background: var(--_19ez2rza);\n  display: flex;\n  flex-direction: column;\n  align-items: center;\n  justify-content: center;\n  gap: 0.5rem;\n  z-index: 10;\n}\n._2a92ce12 {\n  font-size: 2rem;\n  color: var(--_19ez2rz2);\n}\n._2a92ce13 {\n  font-size: 1rem;\n  color: var(--_19ez2rz1);\n  font-weight: 600;\n}\n._2a92ce14 {\n  font-size: 0.8125rem;\n  color: var(--_19ez2rz2);\n}\n._2a92ce15 {\n  position: absolute;\n  inset: 0;\n  background: rgba(0, 0, 0, 0.3);\n  display: flex;\n  align-items: center;\n  justify-content: center;\n  pointer-events: none;\n  z-index: 5;\n}\n._2a92ce17 {\n  width: 32px;\n  height: 32px;\n  border: 3px solid var(--_19ez2rz1f);\n  border-top: 3px solid var(--_19ez2rzm);\n  border-radius: 50%;\n  animation: _2a92ce16 0.8s linear infinite;\n}\n._2a92ce18 {\n  display: flex;\n  flex-direction: column;\n  gap: 0.25rem;\n  padding: 0.4rem 0.5rem;\n  padding-bottom: max(var(--safe-area-bottom, 0px), 0.4rem);\n  background: var(--_19ez2rz1i);\n  border-top: 1px solid var(--_19ez2rz1f);\n  flex-shrink: 0;\n}\n._2a92ce19 {\n  display: flex;\n  gap: 0.3rem;\n  justify-content: center;\n}\n._2a92ce1a {\n  flex: 1 1;\n  padding: 0.45rem 0.25rem;\n  min-width: 0;\n  min-height: var(--min-touch-target, 44px);\n  display: flex;\n  align-items: center;\n  justify-content: center;\n  background: var(--_19ez2rz1k);\n  border: 1px solid var(--_19ez2rz1f);\n  border-bottom: 3px solid var(--_19ez2rz1f);\n  border-radius: 5px;\n  color: var(--_19ez2rz1e);\n  font-family: inherit;\n  font-size: 0.8rem;\n  font-weight: 500;\n  cursor: pointer;\n  -moz-user-select: none;\n       user-select: none;\n  -webkit-user-select: none;\n  touch-action: manipulation;\n  text-align: center;\n  white-space: nowrap;\n  transition: background 0.08s, border-bottom-width 0.08s, transform 0.08s;\n}\n._2a92ce1a:active {\n  background: var(--_19ez2rz1k);\n  border-bottom-width: 1px;\n  transform: translateY(2px);\n}\n._2a92ce1b {\n  background: var(--_19ez2rz11) !important;\n  border-color: var(--_19ez2rz10) !important;\n  color: var(--_19ez2rz12) !important;\n  font-weight: 700;\n  box-shadow: 0 0 0 1px var(--_19ez2rz10);\n}\n._2a92ce1c {\n  position: absolute;\n  top: var(--_19ez2rz2z);\n  left: 50%;\n  transform: translateX(-50%);\n  z-index: 20;\n  padding: var(--_19ez2rz2y) var(--_19ez2rz31);\n  border-radius: 9999px;\n  background: var(--_19ez2rz9);\n  color: var(--_19ez2rz0);\n  font-size: 0.8125rem;\n  text-align: center;\n  white-space: nowrap;\n  pointer-events: none;\n}\n._2a92ce1d {\n  position: absolute;\n  top: var(--_19ez2rz2z);\n  left: 50%;\n  transform: translateX(-50%);\n  z-index: 20;\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2z);\n  padding: var(--_19ez2rz2y) var(--_19ez2rz31);\n  border-radius: 9999px;\n  background: var(--_19ez2rz13);\n  color: var(--_19ez2rz5);\n  font-size: 0.8125rem;\n  white-space: nowrap;\n}\n._2a92ce1d button {\n  background: transparent;\n  border: 1px solid var(--_19ez2rz5);\n  border-radius: 4px;\n  color: var(--_19ez2rz5);\n  padding: 2px 8px;\n  cursor: pointer;\n  font-size: 0.8125rem;\n}\n._2a92ce1d button:hover {\n  background: rgba(255,255,255,0.2);\n}\n._2a92ce1e {\n  position: absolute;\n  top: var(--_19ez2rz2z);\n  left: 50%;\n  transform: translateX(-50%);\n  z-index: 20;\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2z);\n  padding: var(--_19ez2rz2y) var(--_19ez2rz31);\n  border-radius: 9999px;\n  background: var(--_19ez2rz9);\n  color: var(--_19ez2rz0);\n  font-size: 0.8125rem;\n  text-align: center;\n  white-space: nowrap;\n}\n._2a92ce1f {\n  background: transparent;\n  border: 1px solid var(--_19ez2rz0);\n  border-radius: 4px;\n  color: var(--_19ez2rz0);\n  padding: 2px 8px;\n  cursor: pointer;\n  font-size: 0.75rem;\n  pointer-events: auto;\n}\n._2a92ce1g {\n  position: absolute;\n  top: var(--_19ez2rz2z);\n  left: 50%;\n  transform: translateX(-50%);\n  z-index: 20;\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2y);\n  padding: var(--_19ez2rz2y) var(--_19ez2rz31);\n  border-radius: 9999px;\n  background: var(--_19ez2rz9);\n  color: var(--_19ez2rz0);\n  font-size: 0.8125rem;\n  text-align: center;\n  white-space: nowrap;\n  pointer-events: none;\n}\n._2a92ce1h {\n  position: fixed;\n  bottom: var(--_19ez2rz32);\n  left: 50%;\n  transform: translateX(-50%);\n  z-index: 1085;\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz30);\n  max-width: min(90vw, 420px);\n  padding: var(--_19ez2rz2z) var(--_19ez2rz31);\n  border-radius: var(--_19ez2rz37);\n  background: var(--_19ez2rzy);\n  color: var(--_19ez2rzz);\n  font-size: 0.8125rem;\n  line-height: 1.4;\n  box-shadow: 0 2px 8px rgba(0,0,0,0.2);\n}\n._2a92ce1i {\n  flex-shrink: 0;\n  background: transparent;\n  border: 1px solid var(--_19ez2rzz);\n  border-radius: 4px;\n  color: var(--_19ez2rzz);\n  padding: 2px 8px;\n  cursor: pointer;\n  font-size: 0.75rem;\n}\n._2a92ce1j {\n  position: absolute;\n  top: var(--_19ez2rz2z);\n  left: 50%;\n  transform: translateX(-50%);\n  z-index: 15;\n  color: var(--_19ez2rz1j);\n  font-size: 0.75rem;\n  white-space: nowrap;\n  pointer-events: none;\n}\n._2a92cev .xterm-viewport::-webkit-scrollbar {\n  width: 12px;\n  height: 12px;\n}\n._2a92cev .xterm-viewport::-webkit-scrollbar-track {\n  background: var(--_19ez2rz1d);\n}\n._2a92cev .xterm-viewport::-webkit-scrollbar-thumb {\n  background: var(--_19ez2rz1f);\n  border-radius: 6px;\n  border: 2px solid var(--_19ez2rz1d);\n}\n._2a92cev .xterm-viewport::-webkit-scrollbar-thumb:hover {\n  background: var(--_19ez2rz1k);\n}\n._2a92cev .xterm-viewport::-webkit-scrollbar-corner {\n  background: var(--_19ez2rz1d);\n}\n._2a92cev .xterm-selection {\n  background: var(--_19ez2rz17);\n}\n._2a92cev .xterm-helper-textarea {\n  font-size: 16px !important;\n}\n@media screen and (max-width: 768px) {\n  ._2a92ce3 {\n    padding: 0.5rem 0.75rem;\n  }\n  ._2a92ce9 {\n    font-size: 0.7rem;\n  }\n  ._2a92cec {\n    gap: 0.25rem;\n    overflow-x: auto;\n    -webkit-overflow-scrolling: touch;\n    white-space: nowrap;\n    scrollbar-width: none;\n    -ms-overflow-style: none;\n  }\n  ._2a92ced {\n    padding: 0.4rem 0.6rem;\n    font-size: 0.8rem;\n    min-height: var(--min-touch-target, 44px);\n    min-width: var(--min-touch-target, 44px);\n  }\n  ._2a92ceg {\n    display: none;\n  }\n  ._2a92cej {\n    gap: 0.25rem;\n    overflow-x: auto;\n    -webkit-overflow-scrolling: touch;\n    white-space: nowrap;\n    scrollbar-width: none;\n    -ms-overflow-style: none;\n    mask-image: linear-gradient(to right, black calc(100% - 32px), transparent 100%);\n    -webkit-mask-image: linear-gradient(to right, black calc(100% - 32px), transparent 100%);\n  }\n  ._2a92cek {\n    display: none;\n  }\n  ._2a92cel {\n    display: none;\n  }\n  ._2a92cen {\n    display: none;\n  }\n  ._2a92ceo {\n    display: flex;\n    align-items: center;\n    justify-content: center;\n    min-width: var(--min-touch-target, 44px);\n    min-height: var(--min-touch-target, 44px);\n    white-space: nowrap;\n  }\n  ._2a92ceq {\n    display: flex;\n    flex-direction: row-reverse;\n    flex-wrap: wrap;\n    gap: 0.25rem;\n    padding: 0.3rem 0.75rem 0.4rem;\n    background: var(--_19ez2rz7);\n    border-bottom: 1px solid var(--_19ez2rze);\n    flex-shrink: 0;\n  }\n  ._2a92cer {\n    min-height: var(--min-touch-target, 44px);\n    min-width: var(--min-touch-target, 44px);\n  }\n  ._2a92cev {\n    padding: 0;\n    padding-left: var(--safe-area-left, 0px);\n    padding-right: var(--safe-area-right, 0px);\n  }\n}\n@media screen and (max-width: 480px) {\n  ._2a92ced {\n    padding: 0.4rem;\n  }\n  ._2a92cef {\n    display: none;\n  }\n}\n@media (pointer: fine) {\n  ._2a92ceh {\n    display: none;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 33491:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";


const zlib = __webpack_require__(74075);

const bufferUtil = __webpack_require__(95954);
const Limiter = __webpack_require__(29695);
const { kStatusCode } = __webpack_require__(20670);

const FastBuffer = Buffer[Symbol.species];
const TRAILER = Buffer.from([0x00, 0x00, 0xff, 0xff]);
const kPerMessageDeflate = Symbol('permessage-deflate');
const kTotalLength = Symbol('total-length');
const kCallback = Symbol('callback');
const kBuffers = Symbol('buffers');
const kError = Symbol('error');

//
// We limit zlib concurrency, which prevents severe memory fragmentation
// as documented in https://github.com/nodejs/node/issues/8871#issuecomment-250915913
// and https://github.com/websockets/ws/issues/1202
//
// Intentionally global; it's the global thread pool that's an issue.
//
let zlibLimiter;

/**
 * permessage-deflate implementation.
 */
class PerMessageDeflate {
  /**
   * Creates a PerMessageDeflate instance.
   *
   * @param {Object} [options] Configuration options
   * @param {(Boolean|Number)} [options.clientMaxWindowBits] Advertise support
   *     for, or request, a custom client window size
   * @param {Boolean} [options.clientNoContextTakeover=false] Advertise/
   *     acknowledge disabling of client context takeover
   * @param {Number} [options.concurrencyLimit=10] The number of concurrent
   *     calls to zlib
   * @param {(Boolean|Number)} [options.serverMaxWindowBits] Request/confirm the
   *     use of a custom server window size
   * @param {Boolean} [options.serverNoContextTakeover=false] Request/accept
   *     disabling of server context takeover
   * @param {Number} [options.threshold=1024] Size (in bytes) below which
   *     messages should not be compressed if context takeover is disabled
   * @param {Object} [options.zlibDeflateOptions] Options to pass to zlib on
   *     deflate
   * @param {Object} [options.zlibInflateOptions] Options to pass to zlib on
   *     inflate
   * @param {Boolean} [isServer=false] Create the instance in either server or
   *     client mode
   * @param {Number} [maxPayload=0] The maximum allowed message length
   */
  constructor(options, isServer, maxPayload) {
    this._maxPayload = maxPayload | 0;
    this._options = options || {};
    this._threshold =
      this._options.threshold !== undefined ? this._options.threshold : 1024;
    this._isServer = !!isServer;
    this._deflate = null;
    this._inflate = null;

    this.params = null;

    if (!zlibLimiter) {
      const concurrency =
        this._options.concurrencyLimit !== undefined
          ? this._options.concurrencyLimit
          : 10;
      zlibLimiter = new Limiter(concurrency);
    }
  }

  /**
   * @type {String}
   */
  static get extensionName() {
    return 'permessage-deflate';
  }

  /**
   * Create an extension negotiation offer.
   *
   * @return {Object} Extension parameters
   * @public
   */
  offer() {
    const params = {};

    if (this._options.serverNoContextTakeover) {
      params.server_no_context_takeover = true;
    }
    if (this._options.clientNoContextTakeover) {
      params.client_no_context_takeover = true;
    }
    if (this._options.serverMaxWindowBits) {
      params.server_max_window_bits = this._options.serverMaxWindowBits;
    }
    if (this._options.clientMaxWindowBits) {
      params.client_max_window_bits = this._options.clientMaxWindowBits;
    } else if (this._options.clientMaxWindowBits == null) {
      params.client_max_window_bits = true;
    }

    return params;
  }

  /**
   * Accept an extension negotiation offer/response.
   *
   * @param {Array} configurations The extension negotiation offers/reponse
   * @return {Object} Accepted configuration
   * @public
   */
  accept(configurations) {
    configurations = this.normalizeParams(configurations);

    this.params = this._isServer
      ? this.acceptAsServer(configurations)
      : this.acceptAsClient(configurations);

    return this.params;
  }

  /**
   * Releases all resources used by the extension.
   *
   * @public
   */
  cleanup() {
    if (this._inflate) {
      this._inflate.close();
      this._inflate = null;
    }

    if (this._deflate) {
      const callback = this._deflate[kCallback];

      this._deflate.close();
      this._deflate = null;

      if (callback) {
        callback(
          new Error(
            'The deflate stream was closed while data was being processed'
          )
        );
      }
    }
  }

  /**
   *  Accept an extension negotiation offer.
   *
   * @param {Array} offers The extension negotiation offers
   * @return {Object} Accepted configuration
   * @private
   */
  acceptAsServer(offers) {
    const opts = this._options;
    const accepted = offers.find((params) => {
      if (
        (opts.serverNoContextTakeover === false &&
          params.server_no_context_takeover) ||
        (params.server_max_window_bits &&
          (opts.serverMaxWindowBits === false ||
            (typeof opts.serverMaxWindowBits === 'number' &&
              opts.serverMaxWindowBits > params.server_max_window_bits))) ||
        (typeof opts.clientMaxWindowBits === 'number' &&
          !params.client_max_window_bits)
      ) {
        return false;
      }

      return true;
    });

    if (!accepted) {
      throw new Error('None of the extension offers can be accepted');
    }

    if (opts.serverNoContextTakeover) {
      accepted.server_no_context_takeover = true;
    }
    if (opts.clientNoContextTakeover) {
      accepted.client_no_context_takeover = true;
    }
    if (typeof opts.serverMaxWindowBits === 'number') {
      accepted.server_max_window_bits = opts.serverMaxWindowBits;
    }
    if (typeof opts.clientMaxWindowBits === 'number') {
      accepted.client_max_window_bits = opts.clientMaxWindowBits;
    } else if (
      accepted.client_max_window_bits === true ||
      opts.clientMaxWindowBits === false
    ) {
      delete accepted.client_max_window_bits;
    }

    return accepted;
  }

  /**
   * Accept the extension negotiation response.
   *
   * @param {Array} response The extension negotiation response
   * @return {Object} Accepted configuration
   * @private
   */
  acceptAsClient(response) {
    const params = response[0];

    if (
      this._options.clientNoContextTakeover === false &&
      params.client_no_context_takeover
    ) {
      throw new Error('Unexpected parameter "client_no_context_takeover"');
    }

    if (!params.client_max_window_bits) {
      if (typeof this._options.clientMaxWindowBits === 'number') {
        params.client_max_window_bits = this._options.clientMaxWindowBits;
      }
    } else if (
      this._options.clientMaxWindowBits === false ||
      (typeof this._options.clientMaxWindowBits === 'number' &&
        params.client_max_window_bits > this._options.clientMaxWindowBits)
    ) {
      throw new Error(
        'Unexpected or invalid parameter "client_max_window_bits"'
      );
    }

    return params;
  }

  /**
   * Normalize parameters.
   *
   * @param {Array} configurations The extension negotiation offers/reponse
   * @return {Array} The offers/response with normalized parameters
   * @private
   */
  normalizeParams(configurations) {
    configurations.forEach((params) => {
      Object.keys(params).forEach((key) => {
        let value = params[key];

        if (value.length > 1) {
          throw new Error(`Parameter "${key}" must have only a single value`);
        }

        value = value[0];

        if (key === 'client_max_window_bits') {
          if (value !== true) {
            const num = +value;
            if (!Number.isInteger(num) || num < 8 || num > 15) {
              throw new TypeError(
                `Invalid value for parameter "${key}": ${value}`
              );
            }
            value = num;
          } else if (!this._isServer) {
            throw new TypeError(
              `Invalid value for parameter "${key}": ${value}`
            );
          }
        } else if (key === 'server_max_window_bits') {
          const num = +value;
          if (!Number.isInteger(num) || num < 8 || num > 15) {
            throw new TypeError(
              `Invalid value for parameter "${key}": ${value}`
            );
          }
          value = num;
        } else if (
          key === 'client_no_context_takeover' ||
          key === 'server_no_context_takeover'
        ) {
          if (value !== true) {
            throw new TypeError(
              `Invalid value for parameter "${key}": ${value}`
            );
          }
        } else {
          throw new Error(`Unknown parameter "${key}"`);
        }

        params[key] = value;
      });
    });

    return configurations;
  }

  /**
   * Decompress data. Concurrency limited.
   *
   * @param {Buffer} data Compressed data
   * @param {Boolean} fin Specifies whether or not this is the last fragment
   * @param {Function} callback Callback
   * @public
   */
  decompress(data, fin, callback) {
    zlibLimiter.add((done) => {
      this._decompress(data, fin, (err, result) => {
        done();
        callback(err, result);
      });
    });
  }

  /**
   * Compress data. Concurrency limited.
   *
   * @param {(Buffer|String)} data Data to compress
   * @param {Boolean} fin Specifies whether or not this is the last fragment
   * @param {Function} callback Callback
   * @public
   */
  compress(data, fin, callback) {
    zlibLimiter.add((done) => {
      this._compress(data, fin, (err, result) => {
        done();
        callback(err, result);
      });
    });
  }

  /**
   * Decompress data.
   *
   * @param {Buffer} data Compressed data
   * @param {Boolean} fin Specifies whether or not this is the last fragment
   * @param {Function} callback Callback
   * @private
   */
  _decompress(data, fin, callback) {
    const endpoint = this._isServer ? 'client' : 'server';

    if (!this._inflate) {
      const key = `${endpoint}_max_window_bits`;
      const windowBits =
        typeof this.params[key] !== 'number'
          ? zlib.Z_DEFAULT_WINDOWBITS
          : this.params[key];

      this._inflate = zlib.createInflateRaw({
        ...this._options.zlibInflateOptions,
        windowBits
      });
      this._inflate[kPerMessageDeflate] = this;
      this._inflate[kTotalLength] = 0;
      this._inflate[kBuffers] = [];
      this._inflate.on('error', inflateOnError);
      this._inflate.on('data', inflateOnData);
    }

    this._inflate[kCallback] = callback;

    this._inflate.write(data);
    if (fin) this._inflate.write(TRAILER);

    this._inflate.flush(() => {
      const err = this._inflate[kError];

      if (err) {
        this._inflate.close();
        this._inflate = null;
        callback(err);
        return;
      }

      const data = bufferUtil.concat(
        this._inflate[kBuffers],
        this._inflate[kTotalLength]
      );

      if (this._inflate._readableState.endEmitted) {
        this._inflate.close();
        this._inflate = null;
      } else {
        this._inflate[kTotalLength] = 0;
        this._inflate[kBuffers] = [];

        if (fin && this.params[`${endpoint}_no_context_takeover`]) {
          this._inflate.reset();
        }
      }

      callback(null, data);
    });
  }

  /**
   * Compress data.
   *
   * @param {(Buffer|String)} data Data to compress
   * @param {Boolean} fin Specifies whether or not this is the last fragment
   * @param {Function} callback Callback
   * @private
   */
  _compress(data, fin, callback) {
    const endpoint = this._isServer ? 'server' : 'client';

    if (!this._deflate) {
      const key = `${endpoint}_max_window_bits`;
      const windowBits =
        typeof this.params[key] !== 'number'
          ? zlib.Z_DEFAULT_WINDOWBITS
          : this.params[key];

      this._deflate = zlib.createDeflateRaw({
        ...this._options.zlibDeflateOptions,
        windowBits
      });

      this._deflate[kTotalLength] = 0;
      this._deflate[kBuffers] = [];

      this._deflate.on('data', deflateOnData);
    }

    this._deflate[kCallback] = callback;

    this._deflate.write(data);
    this._deflate.flush(zlib.Z_SYNC_FLUSH, () => {
      if (!this._deflate) {
        //
        // The deflate stream was closed while data was being processed.
        //
        return;
      }

      let data = bufferUtil.concat(
        this._deflate[kBuffers],
        this._deflate[kTotalLength]
      );

      if (fin) {
        data = new FastBuffer(data.buffer, data.byteOffset, data.length - 4);
      }

      //
      // Ensure that the callback will not be called again in
      // `PerMessageDeflate#cleanup()`.
      //
      this._deflate[kCallback] = null;

      this._deflate[kTotalLength] = 0;
      this._deflate[kBuffers] = [];

      if (fin && this.params[`${endpoint}_no_context_takeover`]) {
        this._deflate.reset();
      }

      callback(null, data);
    });
  }
}

module.exports = PerMessageDeflate;

/**
 * The listener of the `zlib.DeflateRaw` stream `'data'` event.
 *
 * @param {Buffer} chunk A chunk of data
 * @private
 */
function deflateOnData(chunk) {
  this[kBuffers].push(chunk);
  this[kTotalLength] += chunk.length;
}

/**
 * The listener of the `zlib.InflateRaw` stream `'data'` event.
 *
 * @param {Buffer} chunk A chunk of data
 * @private
 */
function inflateOnData(chunk) {
  this[kTotalLength] += chunk.length;

  if (
    this[kPerMessageDeflate]._maxPayload < 1 ||
    this[kTotalLength] <= this[kPerMessageDeflate]._maxPayload
  ) {
    this[kBuffers].push(chunk);
    return;
  }

  this[kError] = new RangeError('Max payload size exceeded');
  this[kError].code = 'WS_ERR_UNSUPPORTED_MESSAGE_LENGTH';
  this[kError][kStatusCode] = 1009;
  this.removeListener('data', inflateOnData);

  //
  // The choice to employ `zlib.reset()` over `zlib.close()` is dictated by the
  // fact that in Node.js versions prior to 13.10.0, the callback for
  // `zlib.flush()` is not called if `zlib.close()` is used. Utilizing
  // `zlib.reset()` ensures that either the callback is invoked or an error is
  // emitted.
  //
  this.reset();
}

/**
 * The listener of the `zlib.InflateRaw` stream `'error'` event.
 *
 * @param {Error} err The emitted error
 * @private
 */
function inflateOnError(err) {
  //
  // There is no need to call `Zlib#close()` as the handle is automatically
  // closed when an error is emitted.
  //
  this[kPerMessageDeflate]._inflate = null;

  if (this[kError]) {
    this[kCallback](this[kError]);
    return;
  }

  err[kStatusCode] = 1007;
  this[kCallback](err);
}


/***/ }),

/***/ 34411:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "@keyframes _15kbyp70 {\n  0% {\n    opacity: 0;\n  }\n  100% {\n    opacity: 1;\n  }\n}\n._15kbyp71 {\n  position: absolute;\n  right: var(--_19ez2rz2z);\n  z-index: 25;\n  display: inline-flex;\n  align-items: center;\n  justify-content: center;\n  gap: var(--_19ez2rz2y);\n  min-height: 44px;\n  min-width: 44px;\n  width: 11.5rem;\n  padding: var(--_19ez2rz2y) var(--_19ez2rz30);\n  background: var(--_19ez2rzm);\n  color: var(--_19ez2rzq);\n  border: none;\n  border-radius: var(--_19ez2rz39);\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3g);\n  box-shadow: 0 2px 8px rgba(0,0,0,0.3);\n  cursor: pointer;\n  animation: _15kbyp70 120ms ease-out;\n  transition: opacity 120ms ease-out;\n}\n._15kbyp71[data-corner=\"bottom-right\"] {\n  bottom: var(--_19ez2rz2z);\n}\n._15kbyp71[data-corner=\"top-right\"] {\n  top: var(--_19ez2rz2z);\n}\n._15kbyp72 {\n  width: 8px;\n  height: 8px;\n  border-radius: var(--_19ez2rz39);\n  background: var(--_19ez2rzq);\n  flex-shrink: 0;\n}\n@media (prefers-reduced-motion: reduce) {\n  ._15kbyp71 {\n    transition: none;\n    animation: none;\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 38519:
/***/ ((__unused_webpack_module, exports, __webpack_require__) => {

"use strict";
var __webpack_unused_export__;

__webpack_unused_export__ = ({ value: true });
const event_iterator_1 = __webpack_require__(95498);
exports.PP = event_iterator_1.EventIterator;
function stream(evOptions) {
    return new event_iterator_1.EventIterator(queue => {
        this.addListener("data", queue.push);
        this.addListener("end", queue.stop);
        this.addListener("error", queue.fail);
        queue.on("highWater", () => this.pause());
        queue.on("lowWater", () => this.resume());
        return () => {
            this.removeListener("data", queue.push);
            this.removeListener("end", queue.stop);
            this.removeListener("error", queue.fail);
            /* We are no longer interested in any data; attempt to close the stream. */
            if (this.destroy) {
                this.destroy();
            }
            else if (typeof this.close == "function") {
                ;
                this.close();
            }
        };
    }, evOptions);
}
__webpack_unused_export__ = stream;
__webpack_unused_export__ = event_iterator_1.EventIterator;


/***/ }),

/***/ 40731:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Ai: () => (/* binding */ DEFAULT_TERMINAL_CONFIG),
/* harmony export */   Ez: () => (/* binding */ lightTerminalTheme),
/* harmony export */   Gd: () => (/* binding */ darkTerminalTheme),
/* harmony export */   X6: () => (/* binding */ loadTerminalConfig),
/* harmony export */   wo: () => (/* binding */ saveTerminalConfig)
/* harmony export */ });
/* unused harmony export TERMINAL_CONFIG_PRESETS */
/**
 * Terminal configuration system with localStorage persistence
 *
 * Provides user-configurable settings for terminal behavior, appearance,
 * and performance. All settings persist across sessions.
 */ /**
 * Default terminal configuration
 * These values provide good performance and UX for most users
 */ const DEFAULT_TERMINAL_CONFIG = {
    scrollbackLines: 0,
    theme: "dark",
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    enableBell: false,
    cursorStyle: "block",
    cursorBlink: true,
    mouseTracking: "none"
};
/**
 * Configuration presets for different use cases
 */ const TERMINAL_CONFIG_PRESETS = {
    "default": DEFAULT_TERMINAL_CONFIG,
    "high-performance": {
        scrollbackLines: 0,
        cursorBlink: false
    },
    "accessibility": {
        fontSize: 16,
        cursorStyle: "block",
        cursorBlink: false,
        theme: "light"
    }
};
/**
 * Named dark terminal theme for xterm.js
 */ const darkTerminalTheme = {
    background: '#1e1e1e',
    foreground: '#cccccc',
    cursor: '#cccccc',
    cursorAccent: '#1e1e1e',
    selectionBackground: 'rgba(255, 255, 255, 0.3)',
    black: '#000000',
    red: '#cd3131',
    green: '#0dbc79',
    yellow: '#e5e510',
    blue: '#2472c8',
    magenta: '#bc3fbc',
    cyan: '#11a8cd',
    white: '#e5e5e5',
    brightBlack: '#666666',
    brightRed: '#f14c4c',
    brightGreen: '#23d18b',
    brightYellow: '#f5f543',
    brightBlue: '#3b8eea',
    brightMagenta: '#d670d6',
    brightCyan: '#29b8db',
    brightWhite: '#ffffff'
};
/**
 * Named light terminal theme for xterm.js
 */ const lightTerminalTheme = {
    background: '#ffffff',
    foreground: '#1a1a1a',
    cursor: '#333333',
    selectionBackground: 'rgba(0, 112, 243, 0.3)',
    black: '#000000',
    red: '#c0392b',
    green: '#27ae60',
    yellow: '#e67e22',
    blue: '#2980b9',
    magenta: '#8e44ad',
    cyan: '#16a085',
    white: '#bdc3c7',
    brightBlack: '#7f8c8d',
    brightRed: '#e74c3c',
    brightGreen: '#2ecc71',
    brightYellow: '#f39c12',
    brightBlue: '#3498db',
    brightMagenta: '#9b59b6',
    brightCyan: '#1abc9c',
    brightWhite: '#ecf0f1'
};
const STORAGE_KEY = "stapler-squad-terminal-config";
/**
 * Load terminal configuration from localStorage
 * Falls back to default config if not found or invalid
 */ function loadTerminalConfig() {
    if (true) {
        return DEFAULT_TERMINAL_CONFIG;
    }
    try {
        const stored = localStorage.getItem(STORAGE_KEY);
        if (!stored) {
            return DEFAULT_TERMINAL_CONFIG;
        }
        const config = JSON.parse(stored);
        // Validate and merge with defaults
        return {
            ...DEFAULT_TERMINAL_CONFIG,
            ...config,
            // Ensure scrollbackLines is within reasonable bounds (0 = disabled, tmux handles it)
            scrollbackLines: Math.max(0, Math.min(config.scrollbackLines ?? DEFAULT_TERMINAL_CONFIG.scrollbackLines, 100000)),
            // Ensure fontSize is within reasonable bounds
            fontSize: Math.max(8, Math.min(config.fontSize ?? DEFAULT_TERMINAL_CONFIG.fontSize, 32))
        };
    } catch (err) {
        console.error("[terminalConfig] Failed to load config:", err);
        return DEFAULT_TERMINAL_CONFIG;
    }
}
/**
 * Save terminal configuration to localStorage
 */ function saveTerminalConfig(config) {
    if (true) {
        return;
    }
    try {
        const current = loadTerminalConfig();
        const updated = {
            ...current,
            ...config
        };
        localStorage.setItem(STORAGE_KEY, JSON.stringify(updated));
        // Dispatch custom event for other components to react
        window.dispatchEvent(new CustomEvent("terminal-config-changed", {
            detail: updated
        }));
    } catch (err) {
        console.error("[terminalConfig] Failed to save config:", err);
    }
}


/***/ }),

/***/ 47462:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   k: () => (/* binding */ dimensionsEqual),
/* harmony export */   n: () => (/* binding */ isFiniteResizeDimensions)
/* harmony export */ });
/**
 * Shared terminal value types.
 *
 * Hoisted out of XtermTerminal.tsx so useTerminalFlowControl.ts can import
 * ResizeDimensions / isFiniteResizeDimensions without depending on a leaf
 * component (see project_plans/terminal-resize-fit-loop/implementation/plan.md
 * §2 Domain Glossary, architecture-review.md Concern 1).
 */ /**
 * Value type replacing raw positional (cols, rows) pairs wherever a size is
 * held as state (applied / proposed / pending / last-sent dimensions).
 */ /**
 * Canonical Number.isFinite guard on a ResizeDimensions value. Returns false
 * for undefined or for a value with a non-finite cols/rows (e.g. Infinity or
 * NaN), so callers can trust a `true` result before using the dimensions.
 */ function isFiniteResizeDimensions(d) {
    return d !== undefined && Number.isFinite(d.cols) && Number.isFinite(d.rows);
}
/**
 * Structural cols/rows equality. Replaces the hand-rolled `a.cols === b.cols
 * && a.rows === b.rows` comparisons duplicated across the resize pipeline
 * (shouldScheduleFit's two checks, useTerminalFlowControl's resize()
 * value-dedup guard).
 */ function dimensionsEqual(a, b) {
    return a.cols === b.cols && a.rows === b.rows;
}


/***/ }),

/***/ 55696:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";
/* eslint no-unused-vars: ["error", { "varsIgnorePattern": "^Duplex|Readable$", "caughtErrors": "none" }] */



const EventEmitter = __webpack_require__(94735);
const https = __webpack_require__(55591);
const http = __webpack_require__(81630);
const net = __webpack_require__(91645);
const tls = __webpack_require__(34631);
const { randomBytes, createHash } = __webpack_require__(55511);
const { Duplex, Readable } = __webpack_require__(27910);
const { URL } = __webpack_require__(79551);

const PerMessageDeflate = __webpack_require__(33491);
const Receiver = __webpack_require__(69062);
const Sender = __webpack_require__(7026);
const { isBlob } = __webpack_require__(82524);

const {
  BINARY_TYPES,
  EMPTY_BUFFER,
  GUID,
  kForOnEventAttribute,
  kListener,
  kStatusCode,
  kWebSocket,
  NOOP
} = __webpack_require__(20670);
const {
  EventTarget: { addEventListener, removeEventListener }
} = __webpack_require__(94505);
const { format, parse } = __webpack_require__(25270);
const { toBuffer } = __webpack_require__(95954);

const closeTimeout = 30 * 1000;
const kAborted = Symbol('kAborted');
const protocolVersions = [8, 13];
const readyStates = ['CONNECTING', 'OPEN', 'CLOSING', 'CLOSED'];
const subprotocolRegex = /^[!#$%&'*+\-.0-9A-Z^_`|a-z~]+$/;

/**
 * Class representing a WebSocket.
 *
 * @extends EventEmitter
 */
class WebSocket extends EventEmitter {
  /**
   * Create a new `WebSocket`.
   *
   * @param {(String|URL)} address The URL to which to connect
   * @param {(String|String[])} [protocols] The subprotocols
   * @param {Object} [options] Connection options
   */
  constructor(address, protocols, options) {
    super();

    this._binaryType = BINARY_TYPES[0];
    this._closeCode = 1006;
    this._closeFrameReceived = false;
    this._closeFrameSent = false;
    this._closeMessage = EMPTY_BUFFER;
    this._closeTimer = null;
    this._errorEmitted = false;
    this._extensions = {};
    this._paused = false;
    this._protocol = '';
    this._readyState = WebSocket.CONNECTING;
    this._receiver = null;
    this._sender = null;
    this._socket = null;

    if (address !== null) {
      this._bufferedAmount = 0;
      this._isServer = false;
      this._redirects = 0;

      if (protocols === undefined) {
        protocols = [];
      } else if (!Array.isArray(protocols)) {
        if (typeof protocols === 'object' && protocols !== null) {
          options = protocols;
          protocols = [];
        } else {
          protocols = [protocols];
        }
      }

      initAsClient(this, address, protocols, options);
    } else {
      this._autoPong = options.autoPong;
      this._isServer = true;
    }
  }

  /**
   * For historical reasons, the custom "nodebuffer" type is used by the default
   * instead of "blob".
   *
   * @type {String}
   */
  get binaryType() {
    return this._binaryType;
  }

  set binaryType(type) {
    if (!BINARY_TYPES.includes(type)) return;

    this._binaryType = type;

    //
    // Allow to change `binaryType` on the fly.
    //
    if (this._receiver) this._receiver._binaryType = type;
  }

  /**
   * @type {Number}
   */
  get bufferedAmount() {
    if (!this._socket) return this._bufferedAmount;

    return this._socket._writableState.length + this._sender._bufferedBytes;
  }

  /**
   * @type {String}
   */
  get extensions() {
    return Object.keys(this._extensions).join();
  }

  /**
   * @type {Boolean}
   */
  get isPaused() {
    return this._paused;
  }

  /**
   * @type {Function}
   */
  /* istanbul ignore next */
  get onclose() {
    return null;
  }

  /**
   * @type {Function}
   */
  /* istanbul ignore next */
  get onerror() {
    return null;
  }

  /**
   * @type {Function}
   */
  /* istanbul ignore next */
  get onopen() {
    return null;
  }

  /**
   * @type {Function}
   */
  /* istanbul ignore next */
  get onmessage() {
    return null;
  }

  /**
   * @type {String}
   */
  get protocol() {
    return this._protocol;
  }

  /**
   * @type {Number}
   */
  get readyState() {
    return this._readyState;
  }

  /**
   * @type {String}
   */
  get url() {
    return this._url;
  }

  /**
   * Set up the socket and the internal resources.
   *
   * @param {Duplex} socket The network socket between the server and client
   * @param {Buffer} head The first packet of the upgraded stream
   * @param {Object} options Options object
   * @param {Boolean} [options.allowSynchronousEvents=false] Specifies whether
   *     any of the `'message'`, `'ping'`, and `'pong'` events can be emitted
   *     multiple times in the same tick
   * @param {Function} [options.generateMask] The function used to generate the
   *     masking key
   * @param {Number} [options.maxPayload=0] The maximum allowed message size
   * @param {Boolean} [options.skipUTF8Validation=false] Specifies whether or
   *     not to skip UTF-8 validation for text and close messages
   * @private
   */
  setSocket(socket, head, options) {
    const receiver = new Receiver({
      allowSynchronousEvents: options.allowSynchronousEvents,
      binaryType: this.binaryType,
      extensions: this._extensions,
      isServer: this._isServer,
      maxPayload: options.maxPayload,
      skipUTF8Validation: options.skipUTF8Validation
    });

    const sender = new Sender(socket, this._extensions, options.generateMask);

    this._receiver = receiver;
    this._sender = sender;
    this._socket = socket;

    receiver[kWebSocket] = this;
    sender[kWebSocket] = this;
    socket[kWebSocket] = this;

    receiver.on('conclude', receiverOnConclude);
    receiver.on('drain', receiverOnDrain);
    receiver.on('error', receiverOnError);
    receiver.on('message', receiverOnMessage);
    receiver.on('ping', receiverOnPing);
    receiver.on('pong', receiverOnPong);

    sender.onerror = senderOnError;

    //
    // These methods may not be available if `socket` is just a `Duplex`.
    //
    if (socket.setTimeout) socket.setTimeout(0);
    if (socket.setNoDelay) socket.setNoDelay();

    if (head.length > 0) socket.unshift(head);

    socket.on('close', socketOnClose);
    socket.on('data', socketOnData);
    socket.on('end', socketOnEnd);
    socket.on('error', socketOnError);

    this._readyState = WebSocket.OPEN;
    this.emit('open');
  }

  /**
   * Emit the `'close'` event.
   *
   * @private
   */
  emitClose() {
    if (!this._socket) {
      this._readyState = WebSocket.CLOSED;
      this.emit('close', this._closeCode, this._closeMessage);
      return;
    }

    if (this._extensions[PerMessageDeflate.extensionName]) {
      this._extensions[PerMessageDeflate.extensionName].cleanup();
    }

    this._receiver.removeAllListeners();
    this._readyState = WebSocket.CLOSED;
    this.emit('close', this._closeCode, this._closeMessage);
  }

  /**
   * Start a closing handshake.
   *
   *          +----------+   +-----------+   +----------+
   *     - - -|ws.close()|-->|close frame|-->|ws.close()|- - -
   *    |     +----------+   +-----------+   +----------+     |
   *          +----------+   +-----------+         |
   * CLOSING  |ws.close()|<--|close frame|<--+-----+       CLOSING
   *          +----------+   +-----------+   |
   *    |           |                        |   +---+        |
   *                +------------------------+-->|fin| - - - -
   *    |         +---+                      |   +---+
   *     - - - - -|fin|<---------------------+
   *              +---+
   *
   * @param {Number} [code] Status code explaining why the connection is closing
   * @param {(String|Buffer)} [data] The reason why the connection is
   *     closing
   * @public
   */
  close(code, data) {
    if (this.readyState === WebSocket.CLOSED) return;
    if (this.readyState === WebSocket.CONNECTING) {
      const msg = 'WebSocket was closed before the connection was established';
      abortHandshake(this, this._req, msg);
      return;
    }

    if (this.readyState === WebSocket.CLOSING) {
      if (
        this._closeFrameSent &&
        (this._closeFrameReceived || this._receiver._writableState.errorEmitted)
      ) {
        this._socket.end();
      }

      return;
    }

    this._readyState = WebSocket.CLOSING;
    this._sender.close(code, data, !this._isServer, (err) => {
      //
      // This error is handled by the `'error'` listener on the socket. We only
      // want to know if the close frame has been sent here.
      //
      if (err) return;

      this._closeFrameSent = true;

      if (
        this._closeFrameReceived ||
        this._receiver._writableState.errorEmitted
      ) {
        this._socket.end();
      }
    });

    setCloseTimer(this);
  }

  /**
   * Pause the socket.
   *
   * @public
   */
  pause() {
    if (
      this.readyState === WebSocket.CONNECTING ||
      this.readyState === WebSocket.CLOSED
    ) {
      return;
    }

    this._paused = true;
    this._socket.pause();
  }

  /**
   * Send a ping.
   *
   * @param {*} [data] The data to send
   * @param {Boolean} [mask] Indicates whether or not to mask `data`
   * @param {Function} [cb] Callback which is executed when the ping is sent
   * @public
   */
  ping(data, mask, cb) {
    if (this.readyState === WebSocket.CONNECTING) {
      throw new Error('WebSocket is not open: readyState 0 (CONNECTING)');
    }

    if (typeof data === 'function') {
      cb = data;
      data = mask = undefined;
    } else if (typeof mask === 'function') {
      cb = mask;
      mask = undefined;
    }

    if (typeof data === 'number') data = data.toString();

    if (this.readyState !== WebSocket.OPEN) {
      sendAfterClose(this, data, cb);
      return;
    }

    if (mask === undefined) mask = !this._isServer;
    this._sender.ping(data || EMPTY_BUFFER, mask, cb);
  }

  /**
   * Send a pong.
   *
   * @param {*} [data] The data to send
   * @param {Boolean} [mask] Indicates whether or not to mask `data`
   * @param {Function} [cb] Callback which is executed when the pong is sent
   * @public
   */
  pong(data, mask, cb) {
    if (this.readyState === WebSocket.CONNECTING) {
      throw new Error('WebSocket is not open: readyState 0 (CONNECTING)');
    }

    if (typeof data === 'function') {
      cb = data;
      data = mask = undefined;
    } else if (typeof mask === 'function') {
      cb = mask;
      mask = undefined;
    }

    if (typeof data === 'number') data = data.toString();

    if (this.readyState !== WebSocket.OPEN) {
      sendAfterClose(this, data, cb);
      return;
    }

    if (mask === undefined) mask = !this._isServer;
    this._sender.pong(data || EMPTY_BUFFER, mask, cb);
  }

  /**
   * Resume the socket.
   *
   * @public
   */
  resume() {
    if (
      this.readyState === WebSocket.CONNECTING ||
      this.readyState === WebSocket.CLOSED
    ) {
      return;
    }

    this._paused = false;
    if (!this._receiver._writableState.needDrain) this._socket.resume();
  }

  /**
   * Send a data message.
   *
   * @param {*} data The message to send
   * @param {Object} [options] Options object
   * @param {Boolean} [options.binary] Specifies whether `data` is binary or
   *     text
   * @param {Boolean} [options.compress] Specifies whether or not to compress
   *     `data`
   * @param {Boolean} [options.fin=true] Specifies whether the fragment is the
   *     last one
   * @param {Boolean} [options.mask] Specifies whether or not to mask `data`
   * @param {Function} [cb] Callback which is executed when data is written out
   * @public
   */
  send(data, options, cb) {
    if (this.readyState === WebSocket.CONNECTING) {
      throw new Error('WebSocket is not open: readyState 0 (CONNECTING)');
    }

    if (typeof options === 'function') {
      cb = options;
      options = {};
    }

    if (typeof data === 'number') data = data.toString();

    if (this.readyState !== WebSocket.OPEN) {
      sendAfterClose(this, data, cb);
      return;
    }

    const opts = {
      binary: typeof data !== 'string',
      mask: !this._isServer,
      compress: true,
      fin: true,
      ...options
    };

    if (!this._extensions[PerMessageDeflate.extensionName]) {
      opts.compress = false;
    }

    this._sender.send(data || EMPTY_BUFFER, opts, cb);
  }

  /**
   * Forcibly close the connection.
   *
   * @public
   */
  terminate() {
    if (this.readyState === WebSocket.CLOSED) return;
    if (this.readyState === WebSocket.CONNECTING) {
      const msg = 'WebSocket was closed before the connection was established';
      abortHandshake(this, this._req, msg);
      return;
    }

    if (this._socket) {
      this._readyState = WebSocket.CLOSING;
      this._socket.destroy();
    }
  }
}

/**
 * @constant {Number} CONNECTING
 * @memberof WebSocket
 */
Object.defineProperty(WebSocket, 'CONNECTING', {
  enumerable: true,
  value: readyStates.indexOf('CONNECTING')
});

/**
 * @constant {Number} CONNECTING
 * @memberof WebSocket.prototype
 */
Object.defineProperty(WebSocket.prototype, 'CONNECTING', {
  enumerable: true,
  value: readyStates.indexOf('CONNECTING')
});

/**
 * @constant {Number} OPEN
 * @memberof WebSocket
 */
Object.defineProperty(WebSocket, 'OPEN', {
  enumerable: true,
  value: readyStates.indexOf('OPEN')
});

/**
 * @constant {Number} OPEN
 * @memberof WebSocket.prototype
 */
Object.defineProperty(WebSocket.prototype, 'OPEN', {
  enumerable: true,
  value: readyStates.indexOf('OPEN')
});

/**
 * @constant {Number} CLOSING
 * @memberof WebSocket
 */
Object.defineProperty(WebSocket, 'CLOSING', {
  enumerable: true,
  value: readyStates.indexOf('CLOSING')
});

/**
 * @constant {Number} CLOSING
 * @memberof WebSocket.prototype
 */
Object.defineProperty(WebSocket.prototype, 'CLOSING', {
  enumerable: true,
  value: readyStates.indexOf('CLOSING')
});

/**
 * @constant {Number} CLOSED
 * @memberof WebSocket
 */
Object.defineProperty(WebSocket, 'CLOSED', {
  enumerable: true,
  value: readyStates.indexOf('CLOSED')
});

/**
 * @constant {Number} CLOSED
 * @memberof WebSocket.prototype
 */
Object.defineProperty(WebSocket.prototype, 'CLOSED', {
  enumerable: true,
  value: readyStates.indexOf('CLOSED')
});

[
  'binaryType',
  'bufferedAmount',
  'extensions',
  'isPaused',
  'protocol',
  'readyState',
  'url'
].forEach((property) => {
  Object.defineProperty(WebSocket.prototype, property, { enumerable: true });
});

//
// Add the `onopen`, `onerror`, `onclose`, and `onmessage` attributes.
// See https://html.spec.whatwg.org/multipage/comms.html#the-websocket-interface
//
['open', 'error', 'close', 'message'].forEach((method) => {
  Object.defineProperty(WebSocket.prototype, `on${method}`, {
    enumerable: true,
    get() {
      for (const listener of this.listeners(method)) {
        if (listener[kForOnEventAttribute]) return listener[kListener];
      }

      return null;
    },
    set(handler) {
      for (const listener of this.listeners(method)) {
        if (listener[kForOnEventAttribute]) {
          this.removeListener(method, listener);
          break;
        }
      }

      if (typeof handler !== 'function') return;

      this.addEventListener(method, handler, {
        [kForOnEventAttribute]: true
      });
    }
  });
});

WebSocket.prototype.addEventListener = addEventListener;
WebSocket.prototype.removeEventListener = removeEventListener;

module.exports = WebSocket;

/**
 * Initialize a WebSocket client.
 *
 * @param {WebSocket} websocket The client to initialize
 * @param {(String|URL)} address The URL to which to connect
 * @param {Array} protocols The subprotocols
 * @param {Object} [options] Connection options
 * @param {Boolean} [options.allowSynchronousEvents=true] Specifies whether any
 *     of the `'message'`, `'ping'`, and `'pong'` events can be emitted multiple
 *     times in the same tick
 * @param {Boolean} [options.autoPong=true] Specifies whether or not to
 *     automatically send a pong in response to a ping
 * @param {Function} [options.finishRequest] A function which can be used to
 *     customize the headers of each http request before it is sent
 * @param {Boolean} [options.followRedirects=false] Whether or not to follow
 *     redirects
 * @param {Function} [options.generateMask] The function used to generate the
 *     masking key
 * @param {Number} [options.handshakeTimeout] Timeout in milliseconds for the
 *     handshake request
 * @param {Number} [options.maxPayload=104857600] The maximum allowed message
 *     size
 * @param {Number} [options.maxRedirects=10] The maximum number of redirects
 *     allowed
 * @param {String} [options.origin] Value of the `Origin` or
 *     `Sec-WebSocket-Origin` header
 * @param {(Boolean|Object)} [options.perMessageDeflate=true] Enable/disable
 *     permessage-deflate
 * @param {Number} [options.protocolVersion=13] Value of the
 *     `Sec-WebSocket-Version` header
 * @param {Boolean} [options.skipUTF8Validation=false] Specifies whether or
 *     not to skip UTF-8 validation for text and close messages
 * @private
 */
function initAsClient(websocket, address, protocols, options) {
  const opts = {
    allowSynchronousEvents: true,
    autoPong: true,
    protocolVersion: protocolVersions[1],
    maxPayload: 100 * 1024 * 1024,
    skipUTF8Validation: false,
    perMessageDeflate: true,
    followRedirects: false,
    maxRedirects: 10,
    ...options,
    socketPath: undefined,
    hostname: undefined,
    protocol: undefined,
    timeout: undefined,
    method: 'GET',
    host: undefined,
    path: undefined,
    port: undefined
  };

  websocket._autoPong = opts.autoPong;

  if (!protocolVersions.includes(opts.protocolVersion)) {
    throw new RangeError(
      `Unsupported protocol version: ${opts.protocolVersion} ` +
        `(supported versions: ${protocolVersions.join(', ')})`
    );
  }

  let parsedUrl;

  if (address instanceof URL) {
    parsedUrl = address;
  } else {
    try {
      parsedUrl = new URL(address);
    } catch (e) {
      throw new SyntaxError(`Invalid URL: ${address}`);
    }
  }

  if (parsedUrl.protocol === 'http:') {
    parsedUrl.protocol = 'ws:';
  } else if (parsedUrl.protocol === 'https:') {
    parsedUrl.protocol = 'wss:';
  }

  websocket._url = parsedUrl.href;

  const isSecure = parsedUrl.protocol === 'wss:';
  const isIpcUrl = parsedUrl.protocol === 'ws+unix:';
  let invalidUrlMessage;

  if (parsedUrl.protocol !== 'ws:' && !isSecure && !isIpcUrl) {
    invalidUrlMessage =
      'The URL\'s protocol must be one of "ws:", "wss:", ' +
      '"http:", "https:", or "ws+unix:"';
  } else if (isIpcUrl && !parsedUrl.pathname) {
    invalidUrlMessage = "The URL's pathname is empty";
  } else if (parsedUrl.hash) {
    invalidUrlMessage = 'The URL contains a fragment identifier';
  }

  if (invalidUrlMessage) {
    const err = new SyntaxError(invalidUrlMessage);

    if (websocket._redirects === 0) {
      throw err;
    } else {
      emitErrorAndClose(websocket, err);
      return;
    }
  }

  const defaultPort = isSecure ? 443 : 80;
  const key = randomBytes(16).toString('base64');
  const request = isSecure ? https.request : http.request;
  const protocolSet = new Set();
  let perMessageDeflate;

  opts.createConnection =
    opts.createConnection || (isSecure ? tlsConnect : netConnect);
  opts.defaultPort = opts.defaultPort || defaultPort;
  opts.port = parsedUrl.port || defaultPort;
  opts.host = parsedUrl.hostname.startsWith('[')
    ? parsedUrl.hostname.slice(1, -1)
    : parsedUrl.hostname;
  opts.headers = {
    ...opts.headers,
    'Sec-WebSocket-Version': opts.protocolVersion,
    'Sec-WebSocket-Key': key,
    Connection: 'Upgrade',
    Upgrade: 'websocket'
  };
  opts.path = parsedUrl.pathname + parsedUrl.search;
  opts.timeout = opts.handshakeTimeout;

  if (opts.perMessageDeflate) {
    perMessageDeflate = new PerMessageDeflate(
      opts.perMessageDeflate !== true ? opts.perMessageDeflate : {},
      false,
      opts.maxPayload
    );
    opts.headers['Sec-WebSocket-Extensions'] = format({
      [PerMessageDeflate.extensionName]: perMessageDeflate.offer()
    });
  }
  if (protocols.length) {
    for (const protocol of protocols) {
      if (
        typeof protocol !== 'string' ||
        !subprotocolRegex.test(protocol) ||
        protocolSet.has(protocol)
      ) {
        throw new SyntaxError(
          'An invalid or duplicated subprotocol was specified'
        );
      }

      protocolSet.add(protocol);
    }

    opts.headers['Sec-WebSocket-Protocol'] = protocols.join(',');
  }
  if (opts.origin) {
    if (opts.protocolVersion < 13) {
      opts.headers['Sec-WebSocket-Origin'] = opts.origin;
    } else {
      opts.headers.Origin = opts.origin;
    }
  }
  if (parsedUrl.username || parsedUrl.password) {
    opts.auth = `${parsedUrl.username}:${parsedUrl.password}`;
  }

  if (isIpcUrl) {
    const parts = opts.path.split(':');

    opts.socketPath = parts[0];
    opts.path = parts[1];
  }

  let req;

  if (opts.followRedirects) {
    if (websocket._redirects === 0) {
      websocket._originalIpc = isIpcUrl;
      websocket._originalSecure = isSecure;
      websocket._originalHostOrSocketPath = isIpcUrl
        ? opts.socketPath
        : parsedUrl.host;

      const headers = options && options.headers;

      //
      // Shallow copy the user provided options so that headers can be changed
      // without mutating the original object.
      //
      options = { ...options, headers: {} };

      if (headers) {
        for (const [key, value] of Object.entries(headers)) {
          options.headers[key.toLowerCase()] = value;
        }
      }
    } else if (websocket.listenerCount('redirect') === 0) {
      const isSameHost = isIpcUrl
        ? websocket._originalIpc
          ? opts.socketPath === websocket._originalHostOrSocketPath
          : false
        : websocket._originalIpc
          ? false
          : parsedUrl.host === websocket._originalHostOrSocketPath;

      if (!isSameHost || (websocket._originalSecure && !isSecure)) {
        //
        // Match curl 7.77.0 behavior and drop the following headers. These
        // headers are also dropped when following a redirect to a subdomain.
        //
        delete opts.headers.authorization;
        delete opts.headers.cookie;

        if (!isSameHost) delete opts.headers.host;

        opts.auth = undefined;
      }
    }

    //
    // Match curl 7.77.0 behavior and make the first `Authorization` header win.
    // If the `Authorization` header is set, then there is nothing to do as it
    // will take precedence.
    //
    if (opts.auth && !options.headers.authorization) {
      options.headers.authorization =
        'Basic ' + Buffer.from(opts.auth).toString('base64');
    }

    req = websocket._req = request(opts);

    if (websocket._redirects) {
      //
      // Unlike what is done for the `'upgrade'` event, no early exit is
      // triggered here if the user calls `websocket.close()` or
      // `websocket.terminate()` from a listener of the `'redirect'` event. This
      // is because the user can also call `request.destroy()` with an error
      // before calling `websocket.close()` or `websocket.terminate()` and this
      // would result in an error being emitted on the `request` object with no
      // `'error'` event listeners attached.
      //
      websocket.emit('redirect', websocket.url, req);
    }
  } else {
    req = websocket._req = request(opts);
  }

  if (opts.timeout) {
    req.on('timeout', () => {
      abortHandshake(websocket, req, 'Opening handshake has timed out');
    });
  }

  req.on('error', (err) => {
    if (req === null || req[kAborted]) return;

    req = websocket._req = null;
    emitErrorAndClose(websocket, err);
  });

  req.on('response', (res) => {
    const location = res.headers.location;
    const statusCode = res.statusCode;

    if (
      location &&
      opts.followRedirects &&
      statusCode >= 300 &&
      statusCode < 400
    ) {
      if (++websocket._redirects > opts.maxRedirects) {
        abortHandshake(websocket, req, 'Maximum redirects exceeded');
        return;
      }

      req.abort();

      let addr;

      try {
        addr = new URL(location, address);
      } catch (e) {
        const err = new SyntaxError(`Invalid URL: ${location}`);
        emitErrorAndClose(websocket, err);
        return;
      }

      initAsClient(websocket, addr, protocols, options);
    } else if (!websocket.emit('unexpected-response', req, res)) {
      abortHandshake(
        websocket,
        req,
        `Unexpected server response: ${res.statusCode}`
      );
    }
  });

  req.on('upgrade', (res, socket, head) => {
    websocket.emit('upgrade', res);

    //
    // The user may have closed the connection from a listener of the
    // `'upgrade'` event.
    //
    if (websocket.readyState !== WebSocket.CONNECTING) return;

    req = websocket._req = null;

    const upgrade = res.headers.upgrade;

    if (upgrade === undefined || upgrade.toLowerCase() !== 'websocket') {
      abortHandshake(websocket, socket, 'Invalid Upgrade header');
      return;
    }

    const digest = createHash('sha1')
      .update(key + GUID)
      .digest('base64');

    if (res.headers['sec-websocket-accept'] !== digest) {
      abortHandshake(websocket, socket, 'Invalid Sec-WebSocket-Accept header');
      return;
    }

    const serverProt = res.headers['sec-websocket-protocol'];
    let protError;

    if (serverProt !== undefined) {
      if (!protocolSet.size) {
        protError = 'Server sent a subprotocol but none was requested';
      } else if (!protocolSet.has(serverProt)) {
        protError = 'Server sent an invalid subprotocol';
      }
    } else if (protocolSet.size) {
      protError = 'Server sent no subprotocol';
    }

    if (protError) {
      abortHandshake(websocket, socket, protError);
      return;
    }

    if (serverProt) websocket._protocol = serverProt;

    const secWebSocketExtensions = res.headers['sec-websocket-extensions'];

    if (secWebSocketExtensions !== undefined) {
      if (!perMessageDeflate) {
        const message =
          'Server sent a Sec-WebSocket-Extensions header but no extension ' +
          'was requested';
        abortHandshake(websocket, socket, message);
        return;
      }

      let extensions;

      try {
        extensions = parse(secWebSocketExtensions);
      } catch (err) {
        const message = 'Invalid Sec-WebSocket-Extensions header';
        abortHandshake(websocket, socket, message);
        return;
      }

      const extensionNames = Object.keys(extensions);

      if (
        extensionNames.length !== 1 ||
        extensionNames[0] !== PerMessageDeflate.extensionName
      ) {
        const message = 'Server indicated an extension that was not requested';
        abortHandshake(websocket, socket, message);
        return;
      }

      try {
        perMessageDeflate.accept(extensions[PerMessageDeflate.extensionName]);
      } catch (err) {
        const message = 'Invalid Sec-WebSocket-Extensions header';
        abortHandshake(websocket, socket, message);
        return;
      }

      websocket._extensions[PerMessageDeflate.extensionName] =
        perMessageDeflate;
    }

    websocket.setSocket(socket, head, {
      allowSynchronousEvents: opts.allowSynchronousEvents,
      generateMask: opts.generateMask,
      maxPayload: opts.maxPayload,
      skipUTF8Validation: opts.skipUTF8Validation
    });
  });

  if (opts.finishRequest) {
    opts.finishRequest(req, websocket);
  } else {
    req.end();
  }
}

/**
 * Emit the `'error'` and `'close'` events.
 *
 * @param {WebSocket} websocket The WebSocket instance
 * @param {Error} The error to emit
 * @private
 */
function emitErrorAndClose(websocket, err) {
  websocket._readyState = WebSocket.CLOSING;
  //
  // The following assignment is practically useless and is done only for
  // consistency.
  //
  websocket._errorEmitted = true;
  websocket.emit('error', err);
  websocket.emitClose();
}

/**
 * Create a `net.Socket` and initiate a connection.
 *
 * @param {Object} options Connection options
 * @return {net.Socket} The newly created socket used to start the connection
 * @private
 */
function netConnect(options) {
  options.path = options.socketPath;
  return net.connect(options);
}

/**
 * Create a `tls.TLSSocket` and initiate a connection.
 *
 * @param {Object} options Connection options
 * @return {tls.TLSSocket} The newly created socket used to start the connection
 * @private
 */
function tlsConnect(options) {
  options.path = undefined;

  if (!options.servername && options.servername !== '') {
    options.servername = net.isIP(options.host) ? '' : options.host;
  }

  return tls.connect(options);
}

/**
 * Abort the handshake and emit an error.
 *
 * @param {WebSocket} websocket The WebSocket instance
 * @param {(http.ClientRequest|net.Socket|tls.Socket)} stream The request to
 *     abort or the socket to destroy
 * @param {String} message The error message
 * @private
 */
function abortHandshake(websocket, stream, message) {
  websocket._readyState = WebSocket.CLOSING;

  const err = new Error(message);
  Error.captureStackTrace(err, abortHandshake);

  if (stream.setHeader) {
    stream[kAborted] = true;
    stream.abort();

    if (stream.socket && !stream.socket.destroyed) {
      //
      // On Node.js >= 14.3.0 `request.abort()` does not destroy the socket if
      // called after the request completed. See
      // https://github.com/websockets/ws/issues/1869.
      //
      stream.socket.destroy();
    }

    process.nextTick(emitErrorAndClose, websocket, err);
  } else {
    stream.destroy(err);
    stream.once('error', websocket.emit.bind(websocket, 'error'));
    stream.once('close', websocket.emitClose.bind(websocket));
  }
}

/**
 * Handle cases where the `ping()`, `pong()`, or `send()` methods are called
 * when the `readyState` attribute is `CLOSING` or `CLOSED`.
 *
 * @param {WebSocket} websocket The WebSocket instance
 * @param {*} [data] The data to send
 * @param {Function} [cb] Callback
 * @private
 */
function sendAfterClose(websocket, data, cb) {
  if (data) {
    const length = isBlob(data) ? data.size : toBuffer(data).length;

    //
    // The `_bufferedAmount` property is used only when the peer is a client and
    // the opening handshake fails. Under these circumstances, in fact, the
    // `setSocket()` method is not called, so the `_socket` and `_sender`
    // properties are set to `null`.
    //
    if (websocket._socket) websocket._sender._bufferedBytes += length;
    else websocket._bufferedAmount += length;
  }

  if (cb) {
    const err = new Error(
      `WebSocket is not open: readyState ${websocket.readyState} ` +
        `(${readyStates[websocket.readyState]})`
    );
    process.nextTick(cb, err);
  }
}

/**
 * The listener of the `Receiver` `'conclude'` event.
 *
 * @param {Number} code The status code
 * @param {Buffer} reason The reason for closing
 * @private
 */
function receiverOnConclude(code, reason) {
  const websocket = this[kWebSocket];

  websocket._closeFrameReceived = true;
  websocket._closeMessage = reason;
  websocket._closeCode = code;

  if (websocket._socket[kWebSocket] === undefined) return;

  websocket._socket.removeListener('data', socketOnData);
  process.nextTick(resume, websocket._socket);

  if (code === 1005) websocket.close();
  else websocket.close(code, reason);
}

/**
 * The listener of the `Receiver` `'drain'` event.
 *
 * @private
 */
function receiverOnDrain() {
  const websocket = this[kWebSocket];

  if (!websocket.isPaused) websocket._socket.resume();
}

/**
 * The listener of the `Receiver` `'error'` event.
 *
 * @param {(RangeError|Error)} err The emitted error
 * @private
 */
function receiverOnError(err) {
  const websocket = this[kWebSocket];

  if (websocket._socket[kWebSocket] !== undefined) {
    websocket._socket.removeListener('data', socketOnData);

    //
    // On Node.js < 14.0.0 the `'error'` event is emitted synchronously. See
    // https://github.com/websockets/ws/issues/1940.
    //
    process.nextTick(resume, websocket._socket);

    websocket.close(err[kStatusCode]);
  }

  if (!websocket._errorEmitted) {
    websocket._errorEmitted = true;
    websocket.emit('error', err);
  }
}

/**
 * The listener of the `Receiver` `'finish'` event.
 *
 * @private
 */
function receiverOnFinish() {
  this[kWebSocket].emitClose();
}

/**
 * The listener of the `Receiver` `'message'` event.
 *
 * @param {Buffer|ArrayBuffer|Buffer[])} data The message
 * @param {Boolean} isBinary Specifies whether the message is binary or not
 * @private
 */
function receiverOnMessage(data, isBinary) {
  this[kWebSocket].emit('message', data, isBinary);
}

/**
 * The listener of the `Receiver` `'ping'` event.
 *
 * @param {Buffer} data The data included in the ping frame
 * @private
 */
function receiverOnPing(data) {
  const websocket = this[kWebSocket];

  if (websocket._autoPong) websocket.pong(data, !this._isServer, NOOP);
  websocket.emit('ping', data);
}

/**
 * The listener of the `Receiver` `'pong'` event.
 *
 * @param {Buffer} data The data included in the pong frame
 * @private
 */
function receiverOnPong(data) {
  this[kWebSocket].emit('pong', data);
}

/**
 * Resume a readable stream
 *
 * @param {Readable} stream The readable stream
 * @private
 */
function resume(stream) {
  stream.resume();
}

/**
 * The `Sender` error event handler.
 *
 * @param {Error} The error
 * @private
 */
function senderOnError(err) {
  const websocket = this[kWebSocket];

  if (websocket.readyState === WebSocket.CLOSED) return;
  if (websocket.readyState === WebSocket.OPEN) {
    websocket._readyState = WebSocket.CLOSING;
    setCloseTimer(websocket);
  }

  //
  // `socket.end()` is used instead of `socket.destroy()` to allow the other
  // peer to finish sending queued data. There is no need to set a timer here
  // because `CLOSING` means that it is already set or not needed.
  //
  this._socket.end();

  if (!websocket._errorEmitted) {
    websocket._errorEmitted = true;
    websocket.emit('error', err);
  }
}

/**
 * Set a timer to destroy the underlying raw socket of a WebSocket.
 *
 * @param {WebSocket} websocket The WebSocket instance
 * @private
 */
function setCloseTimer(websocket) {
  websocket._closeTimer = setTimeout(
    websocket._socket.destroy.bind(websocket._socket),
    closeTimeout
  );
}

/**
 * The listener of the socket `'close'` event.
 *
 * @private
 */
function socketOnClose() {
  const websocket = this[kWebSocket];

  this.removeListener('close', socketOnClose);
  this.removeListener('data', socketOnData);
  this.removeListener('end', socketOnEnd);

  websocket._readyState = WebSocket.CLOSING;

  let chunk;

  //
  // The close frame might not have been received or the `'end'` event emitted,
  // for example, if the socket was destroyed due to an error. Ensure that the
  // `receiver` stream is closed after writing any remaining buffered data to
  // it. If the readable side of the socket is in flowing mode then there is no
  // buffered data as everything has been already written and `readable.read()`
  // will return `null`. If instead, the socket is paused, any possible buffered
  // data will be read as a single chunk.
  //
  if (
    !this._readableState.endEmitted &&
    !websocket._closeFrameReceived &&
    !websocket._receiver._writableState.errorEmitted &&
    (chunk = websocket._socket.read()) !== null
  ) {
    websocket._receiver.write(chunk);
  }

  websocket._receiver.end();

  this[kWebSocket] = undefined;

  clearTimeout(websocket._closeTimer);

  if (
    websocket._receiver._writableState.finished ||
    websocket._receiver._writableState.errorEmitted
  ) {
    websocket.emitClose();
  } else {
    websocket._receiver.on('error', receiverOnFinish);
    websocket._receiver.on('finish', receiverOnFinish);
  }
}

/**
 * The listener of the socket `'data'` event.
 *
 * @param {Buffer} chunk A chunk of data
 * @private
 */
function socketOnData(chunk) {
  if (!this[kWebSocket]._receiver.write(chunk)) {
    this.pause();
  }
}

/**
 * The listener of the socket `'end'` event.
 *
 * @private
 */
function socketOnEnd() {
  const websocket = this[kWebSocket];

  websocket._readyState = WebSocket.CLOSING;
  websocket._receiver.end();
  this.end();
}

/**
 * The listener of the socket `'error'` event.
 *
 * @private
 */
function socketOnError() {
  const websocket = this[kWebSocket];

  this.removeListener('error', socketOnError);
  this.on('error', NOOP);

  if (websocket) {
    websocket._readyState = WebSocket.CLOSING;
    this.destroy();
  }
}


/***/ }),

/***/ 65176:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, ".sxbzrg0 {\n  position: absolute;\n  top: 0;\n  left: 0;\n  right: 0;\n  z-index: 26;\n  display: flex;\n  flex-direction: column;\n  max-height: 50%;\n  background: var(--_19ez2rz7);\n  border-bottom: 1px solid var(--_19ez2rze);\n}\n.sxbzrg1 {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2z);\n  padding: var(--_19ez2rz2z);\n}\n.sxbzrg2 {\n  flex: 1 1;\n  min-width: 0;\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  font-size: var(--_19ez2rz3b);\n}\n.sxbzrg3 {\n  display: flex;\n  align-items: center;\n  gap: var(--_19ez2rz2y);\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz1);\n  white-space: nowrap;\n}\n.sxbzrg4 {\n  padding: 0 var(--_19ez2rz2z) var(--_19ez2rz2y);\n  font-size: var(--_19ez2rz3b);\n  color: var(--_19ez2rz1);\n}\n.sxbzrg5 {\n  margin: 0;\n  padding: 0;\n  list-style: none;\n  overflow-y: auto;\n  font-family: monospace;\n  font-size: var(--_19ez2rz3b);\n}\n.sxbzrg6 {\n  display: flex;\n  gap: var(--_19ez2rz2z);\n  padding: 0 var(--_19ez2rz2z);\n  white-space: pre-wrap;\n  word-break: break-all;\n}\n.sxbzrg7 {\n  flex-shrink: 0;\n  min-width: 5ch;\n  text-align: right;\n  color: var(--_19ez2rz2);\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 67535:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   t: () => (/* binding */ useInputModeOverride)
/* harmony export */ });
/* unused harmony export INPUT_MODE_OVERRIDE_KEY */
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);
/* __next_internal_client_entry_do_not_use__ INPUT_MODE_OVERRIDE_KEY,useInputModeOverride auto */ 
const INPUT_MODE_OVERRIDE_KEY = "stapler-squad:input-mode-override";
/**
 * Reads and persists the user's override for mouse+keyboard-on-mobile detection.
 *
 * "auto" (default) lets the terminal toolbar decide based on live hardware
 * signals (matchMedia hover/pointer:fine via useViewport().hasFinePointer).
 * "desktop" and "touch" force that decision regardless of what the hardware
 * reports, for the rare case detection gets it wrong (e.g. some Android
 * keyboard cases, or a user who prefers the expanded toolbar despite a mouse).
 */ function useInputModeOverride() {
    // Read synchronously via a lazy initializer (not a mount effect) — a caller that
    // derives a one-time default from inputModeOverride on its own mount effect would
    // otherwise see the "auto" fallback before an async localStorage-read effect here
    // resolved the persisted value, and could apply the wrong default irreversibly.
    const [inputModeOverride, setInputModeOverrideState] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(()=>{
        if (typeof localStorage === "undefined") return "auto";
        try {
            const stored = localStorage.getItem(INPUT_MODE_OVERRIDE_KEY);
            if (stored === "auto" || stored === "desktop" || stored === "touch") {
                return stored;
            }
        } catch  {}
        return "auto";
    });
    const setInputModeOverride = (next)=>{
        setInputModeOverrideState(next);
        try {
            localStorage.setItem(INPUT_MODE_OVERRIDE_KEY, next);
        } catch  {}
    };
    return {
        inputModeOverride,
        setInputModeOverride
    };
}


/***/ }),

/***/ 68226:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";
/* eslint no-unused-vars: ["error", { "varsIgnorePattern": "^Duplex$", "caughtErrors": "none" }] */



const EventEmitter = __webpack_require__(94735);
const http = __webpack_require__(81630);
const { Duplex } = __webpack_require__(27910);
const { createHash } = __webpack_require__(55511);

const extension = __webpack_require__(25270);
const PerMessageDeflate = __webpack_require__(33491);
const subprotocol = __webpack_require__(12873);
const WebSocket = __webpack_require__(55696);
const { GUID, kWebSocket } = __webpack_require__(20670);

const keyRegex = /^[+/0-9A-Za-z]{22}==$/;

const RUNNING = 0;
const CLOSING = 1;
const CLOSED = 2;

/**
 * Class representing a WebSocket server.
 *
 * @extends EventEmitter
 */
class WebSocketServer extends EventEmitter {
  /**
   * Create a `WebSocketServer` instance.
   *
   * @param {Object} options Configuration options
   * @param {Boolean} [options.allowSynchronousEvents=true] Specifies whether
   *     any of the `'message'`, `'ping'`, and `'pong'` events can be emitted
   *     multiple times in the same tick
   * @param {Boolean} [options.autoPong=true] Specifies whether or not to
   *     automatically send a pong in response to a ping
   * @param {Number} [options.backlog=511] The maximum length of the queue of
   *     pending connections
   * @param {Boolean} [options.clientTracking=true] Specifies whether or not to
   *     track clients
   * @param {Function} [options.handleProtocols] A hook to handle protocols
   * @param {String} [options.host] The hostname where to bind the server
   * @param {Number} [options.maxPayload=104857600] The maximum allowed message
   *     size
   * @param {Boolean} [options.noServer=false] Enable no server mode
   * @param {String} [options.path] Accept only connections matching this path
   * @param {(Boolean|Object)} [options.perMessageDeflate=false] Enable/disable
   *     permessage-deflate
   * @param {Number} [options.port] The port where to bind the server
   * @param {(http.Server|https.Server)} [options.server] A pre-created HTTP/S
   *     server to use
   * @param {Boolean} [options.skipUTF8Validation=false] Specifies whether or
   *     not to skip UTF-8 validation for text and close messages
   * @param {Function} [options.verifyClient] A hook to reject connections
   * @param {Function} [options.WebSocket=WebSocket] Specifies the `WebSocket`
   *     class to use. It must be the `WebSocket` class or class that extends it
   * @param {Function} [callback] A listener for the `listening` event
   */
  constructor(options, callback) {
    super();

    options = {
      allowSynchronousEvents: true,
      autoPong: true,
      maxPayload: 100 * 1024 * 1024,
      skipUTF8Validation: false,
      perMessageDeflate: false,
      handleProtocols: null,
      clientTracking: true,
      verifyClient: null,
      noServer: false,
      backlog: null, // use default (511 as implemented in net.js)
      server: null,
      host: null,
      path: null,
      port: null,
      WebSocket,
      ...options
    };

    if (
      (options.port == null && !options.server && !options.noServer) ||
      (options.port != null && (options.server || options.noServer)) ||
      (options.server && options.noServer)
    ) {
      throw new TypeError(
        'One and only one of the "port", "server", or "noServer" options ' +
          'must be specified'
      );
    }

    if (options.port != null) {
      this._server = http.createServer((req, res) => {
        const body = http.STATUS_CODES[426];

        res.writeHead(426, {
          'Content-Length': body.length,
          'Content-Type': 'text/plain'
        });
        res.end(body);
      });
      this._server.listen(
        options.port,
        options.host,
        options.backlog,
        callback
      );
    } else if (options.server) {
      this._server = options.server;
    }

    if (this._server) {
      const emitConnection = this.emit.bind(this, 'connection');

      this._removeListeners = addListeners(this._server, {
        listening: this.emit.bind(this, 'listening'),
        error: this.emit.bind(this, 'error'),
        upgrade: (req, socket, head) => {
          this.handleUpgrade(req, socket, head, emitConnection);
        }
      });
    }

    if (options.perMessageDeflate === true) options.perMessageDeflate = {};
    if (options.clientTracking) {
      this.clients = new Set();
      this._shouldEmitClose = false;
    }

    this.options = options;
    this._state = RUNNING;
  }

  /**
   * Returns the bound address, the address family name, and port of the server
   * as reported by the operating system if listening on an IP socket.
   * If the server is listening on a pipe or UNIX domain socket, the name is
   * returned as a string.
   *
   * @return {(Object|String|null)} The address of the server
   * @public
   */
  address() {
    if (this.options.noServer) {
      throw new Error('The server is operating in "noServer" mode');
    }

    if (!this._server) return null;
    return this._server.address();
  }

  /**
   * Stop the server from accepting new connections and emit the `'close'` event
   * when all existing connections are closed.
   *
   * @param {Function} [cb] A one-time listener for the `'close'` event
   * @public
   */
  close(cb) {
    if (this._state === CLOSED) {
      if (cb) {
        this.once('close', () => {
          cb(new Error('The server is not running'));
        });
      }

      process.nextTick(emitClose, this);
      return;
    }

    if (cb) this.once('close', cb);

    if (this._state === CLOSING) return;
    this._state = CLOSING;

    if (this.options.noServer || this.options.server) {
      if (this._server) {
        this._removeListeners();
        this._removeListeners = this._server = null;
      }

      if (this.clients) {
        if (!this.clients.size) {
          process.nextTick(emitClose, this);
        } else {
          this._shouldEmitClose = true;
        }
      } else {
        process.nextTick(emitClose, this);
      }
    } else {
      const server = this._server;

      this._removeListeners();
      this._removeListeners = this._server = null;

      //
      // The HTTP/S server was created internally. Close it, and rely on its
      // `'close'` event.
      //
      server.close(() => {
        emitClose(this);
      });
    }
  }

  /**
   * See if a given request should be handled by this server instance.
   *
   * @param {http.IncomingMessage} req Request object to inspect
   * @return {Boolean} `true` if the request is valid, else `false`
   * @public
   */
  shouldHandle(req) {
    if (this.options.path) {
      const index = req.url.indexOf('?');
      const pathname = index !== -1 ? req.url.slice(0, index) : req.url;

      if (pathname !== this.options.path) return false;
    }

    return true;
  }

  /**
   * Handle a HTTP Upgrade request.
   *
   * @param {http.IncomingMessage} req The request object
   * @param {Duplex} socket The network socket between the server and client
   * @param {Buffer} head The first packet of the upgraded stream
   * @param {Function} cb Callback
   * @public
   */
  handleUpgrade(req, socket, head, cb) {
    socket.on('error', socketOnError);

    const key = req.headers['sec-websocket-key'];
    const upgrade = req.headers.upgrade;
    const version = +req.headers['sec-websocket-version'];

    if (req.method !== 'GET') {
      const message = 'Invalid HTTP method';
      abortHandshakeOrEmitwsClientError(this, req, socket, 405, message);
      return;
    }

    if (upgrade === undefined || upgrade.toLowerCase() !== 'websocket') {
      const message = 'Invalid Upgrade header';
      abortHandshakeOrEmitwsClientError(this, req, socket, 400, message);
      return;
    }

    if (key === undefined || !keyRegex.test(key)) {
      const message = 'Missing or invalid Sec-WebSocket-Key header';
      abortHandshakeOrEmitwsClientError(this, req, socket, 400, message);
      return;
    }

    if (version !== 13 && version !== 8) {
      const message = 'Missing or invalid Sec-WebSocket-Version header';
      abortHandshakeOrEmitwsClientError(this, req, socket, 400, message, {
        'Sec-WebSocket-Version': '13, 8'
      });
      return;
    }

    if (!this.shouldHandle(req)) {
      abortHandshake(socket, 400);
      return;
    }

    const secWebSocketProtocol = req.headers['sec-websocket-protocol'];
    let protocols = new Set();

    if (secWebSocketProtocol !== undefined) {
      try {
        protocols = subprotocol.parse(secWebSocketProtocol);
      } catch (err) {
        const message = 'Invalid Sec-WebSocket-Protocol header';
        abortHandshakeOrEmitwsClientError(this, req, socket, 400, message);
        return;
      }
    }

    const secWebSocketExtensions = req.headers['sec-websocket-extensions'];
    const extensions = {};

    if (
      this.options.perMessageDeflate &&
      secWebSocketExtensions !== undefined
    ) {
      const perMessageDeflate = new PerMessageDeflate(
        this.options.perMessageDeflate,
        true,
        this.options.maxPayload
      );

      try {
        const offers = extension.parse(secWebSocketExtensions);

        if (offers[PerMessageDeflate.extensionName]) {
          perMessageDeflate.accept(offers[PerMessageDeflate.extensionName]);
          extensions[PerMessageDeflate.extensionName] = perMessageDeflate;
        }
      } catch (err) {
        const message =
          'Invalid or unacceptable Sec-WebSocket-Extensions header';
        abortHandshakeOrEmitwsClientError(this, req, socket, 400, message);
        return;
      }
    }

    //
    // Optionally call external client verification handler.
    //
    if (this.options.verifyClient) {
      const info = {
        origin:
          req.headers[`${version === 8 ? 'sec-websocket-origin' : 'origin'}`],
        secure: !!(req.socket.authorized || req.socket.encrypted),
        req
      };

      if (this.options.verifyClient.length === 2) {
        this.options.verifyClient(info, (verified, code, message, headers) => {
          if (!verified) {
            return abortHandshake(socket, code || 401, message, headers);
          }

          this.completeUpgrade(
            extensions,
            key,
            protocols,
            req,
            socket,
            head,
            cb
          );
        });
        return;
      }

      if (!this.options.verifyClient(info)) return abortHandshake(socket, 401);
    }

    this.completeUpgrade(extensions, key, protocols, req, socket, head, cb);
  }

  /**
   * Upgrade the connection to WebSocket.
   *
   * @param {Object} extensions The accepted extensions
   * @param {String} key The value of the `Sec-WebSocket-Key` header
   * @param {Set} protocols The subprotocols
   * @param {http.IncomingMessage} req The request object
   * @param {Duplex} socket The network socket between the server and client
   * @param {Buffer} head The first packet of the upgraded stream
   * @param {Function} cb Callback
   * @throws {Error} If called more than once with the same socket
   * @private
   */
  completeUpgrade(extensions, key, protocols, req, socket, head, cb) {
    //
    // Destroy the socket if the client has already sent a FIN packet.
    //
    if (!socket.readable || !socket.writable) return socket.destroy();

    if (socket[kWebSocket]) {
      throw new Error(
        'server.handleUpgrade() was called more than once with the same ' +
          'socket, possibly due to a misconfiguration'
      );
    }

    if (this._state > RUNNING) return abortHandshake(socket, 503);

    const digest = createHash('sha1')
      .update(key + GUID)
      .digest('base64');

    const headers = [
      'HTTP/1.1 101 Switching Protocols',
      'Upgrade: websocket',
      'Connection: Upgrade',
      `Sec-WebSocket-Accept: ${digest}`
    ];

    const ws = new this.options.WebSocket(null, undefined, this.options);

    if (protocols.size) {
      //
      // Optionally call external protocol selection handler.
      //
      const protocol = this.options.handleProtocols
        ? this.options.handleProtocols(protocols, req)
        : protocols.values().next().value;

      if (protocol) {
        headers.push(`Sec-WebSocket-Protocol: ${protocol}`);
        ws._protocol = protocol;
      }
    }

    if (extensions[PerMessageDeflate.extensionName]) {
      const params = extensions[PerMessageDeflate.extensionName].params;
      const value = extension.format({
        [PerMessageDeflate.extensionName]: [params]
      });
      headers.push(`Sec-WebSocket-Extensions: ${value}`);
      ws._extensions = extensions;
    }

    //
    // Allow external modification/inspection of handshake headers.
    //
    this.emit('headers', headers, req);

    socket.write(headers.concat('\r\n').join('\r\n'));
    socket.removeListener('error', socketOnError);

    ws.setSocket(socket, head, {
      allowSynchronousEvents: this.options.allowSynchronousEvents,
      maxPayload: this.options.maxPayload,
      skipUTF8Validation: this.options.skipUTF8Validation
    });

    if (this.clients) {
      this.clients.add(ws);
      ws.on('close', () => {
        this.clients.delete(ws);

        if (this._shouldEmitClose && !this.clients.size) {
          process.nextTick(emitClose, this);
        }
      });
    }

    cb(ws, req);
  }
}

module.exports = WebSocketServer;

/**
 * Add event listeners on an `EventEmitter` using a map of <event, listener>
 * pairs.
 *
 * @param {EventEmitter} server The event emitter
 * @param {Object.<String, Function>} map The listeners to add
 * @return {Function} A function that will remove the added listeners when
 *     called
 * @private
 */
function addListeners(server, map) {
  for (const event of Object.keys(map)) server.on(event, map[event]);

  return function removeListeners() {
    for (const event of Object.keys(map)) {
      server.removeListener(event, map[event]);
    }
  };
}

/**
 * Emit a `'close'` event on an `EventEmitter`.
 *
 * @param {EventEmitter} server The event emitter
 * @private
 */
function emitClose(server) {
  server._state = CLOSED;
  server.emit('close');
}

/**
 * Handle socket errors.
 *
 * @private
 */
function socketOnError() {
  this.destroy();
}

/**
 * Close the connection when preconditions are not fulfilled.
 *
 * @param {Duplex} socket The socket of the upgrade request
 * @param {Number} code The HTTP response status code
 * @param {String} [message] The HTTP response body
 * @param {Object} [headers] Additional HTTP response headers
 * @private
 */
function abortHandshake(socket, code, message, headers) {
  //
  // The socket is writable unless the user destroyed or ended it before calling
  // `server.handleUpgrade()` or in the `verifyClient` function, which is a user
  // error. Handling this does not make much sense as the worst that can happen
  // is that some of the data written by the user might be discarded due to the
  // call to `socket.end()` below, which triggers an `'error'` event that in
  // turn causes the socket to be destroyed.
  //
  message = message || http.STATUS_CODES[code];
  headers = {
    Connection: 'close',
    'Content-Type': 'text/html',
    'Content-Length': Buffer.byteLength(message),
    ...headers
  };

  socket.once('finish', socket.destroy);

  socket.end(
    `HTTP/1.1 ${code} ${http.STATUS_CODES[code]}\r\n` +
      Object.keys(headers)
        .map((h) => `${h}: ${headers[h]}`)
        .join('\r\n') +
      '\r\n\r\n' +
      message
  );
}

/**
 * Emit a `'wsClientError'` event on a `WebSocketServer` if there is at least
 * one listener for it, otherwise call `abortHandshake()`.
 *
 * @param {WebSocketServer} server The WebSocket server
 * @param {http.IncomingMessage} req The request object
 * @param {Duplex} socket The socket of the upgrade request
 * @param {Number} code The HTTP response status code
 * @param {String} message The HTTP response body
 * @param {Object} [headers] The HTTP response headers
 * @private
 */
function abortHandshakeOrEmitwsClientError(
  server,
  req,
  socket,
  code,
  message,
  headers
) {
  if (server.listenerCount('wsClientError')) {
    const err = new Error(message);
    Error.captureStackTrace(err, abortHandshakeOrEmitwsClientError);

    server.emit('wsClientError', err, socket, req);
  } else {
    abortHandshake(socket, code, message, headers);
  }
}


/***/ }),

/***/ 69062:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";


const { Writable } = __webpack_require__(27910);

const PerMessageDeflate = __webpack_require__(33491);
const {
  BINARY_TYPES,
  EMPTY_BUFFER,
  kStatusCode,
  kWebSocket
} = __webpack_require__(20670);
const { concat, toArrayBuffer, unmask } = __webpack_require__(95954);
const { isValidStatusCode, isValidUTF8 } = __webpack_require__(82524);

const FastBuffer = Buffer[Symbol.species];

const GET_INFO = 0;
const GET_PAYLOAD_LENGTH_16 = 1;
const GET_PAYLOAD_LENGTH_64 = 2;
const GET_MASK = 3;
const GET_DATA = 4;
const INFLATING = 5;
const DEFER_EVENT = 6;

/**
 * HyBi Receiver implementation.
 *
 * @extends Writable
 */
class Receiver extends Writable {
  /**
   * Creates a Receiver instance.
   *
   * @param {Object} [options] Options object
   * @param {Boolean} [options.allowSynchronousEvents=true] Specifies whether
   *     any of the `'message'`, `'ping'`, and `'pong'` events can be emitted
   *     multiple times in the same tick
   * @param {String} [options.binaryType=nodebuffer] The type for binary data
   * @param {Object} [options.extensions] An object containing the negotiated
   *     extensions
   * @param {Boolean} [options.isServer=false] Specifies whether to operate in
   *     client or server mode
   * @param {Number} [options.maxPayload=0] The maximum allowed message length
   * @param {Boolean} [options.skipUTF8Validation=false] Specifies whether or
   *     not to skip UTF-8 validation for text and close messages
   */
  constructor(options = {}) {
    super();

    this._allowSynchronousEvents =
      options.allowSynchronousEvents !== undefined
        ? options.allowSynchronousEvents
        : true;
    this._binaryType = options.binaryType || BINARY_TYPES[0];
    this._extensions = options.extensions || {};
    this._isServer = !!options.isServer;
    this._maxPayload = options.maxPayload | 0;
    this._skipUTF8Validation = !!options.skipUTF8Validation;
    this[kWebSocket] = undefined;

    this._bufferedBytes = 0;
    this._buffers = [];

    this._compressed = false;
    this._payloadLength = 0;
    this._mask = undefined;
    this._fragmented = 0;
    this._masked = false;
    this._fin = false;
    this._opcode = 0;

    this._totalPayloadLength = 0;
    this._messageLength = 0;
    this._fragments = [];

    this._errored = false;
    this._loop = false;
    this._state = GET_INFO;
  }

  /**
   * Implements `Writable.prototype._write()`.
   *
   * @param {Buffer} chunk The chunk of data to write
   * @param {String} encoding The character encoding of `chunk`
   * @param {Function} cb Callback
   * @private
   */
  _write(chunk, encoding, cb) {
    if (this._opcode === 0x08 && this._state == GET_INFO) return cb();

    this._bufferedBytes += chunk.length;
    this._buffers.push(chunk);
    this.startLoop(cb);
  }

  /**
   * Consumes `n` bytes from the buffered data.
   *
   * @param {Number} n The number of bytes to consume
   * @return {Buffer} The consumed bytes
   * @private
   */
  consume(n) {
    this._bufferedBytes -= n;

    if (n === this._buffers[0].length) return this._buffers.shift();

    if (n < this._buffers[0].length) {
      const buf = this._buffers[0];
      this._buffers[0] = new FastBuffer(
        buf.buffer,
        buf.byteOffset + n,
        buf.length - n
      );

      return new FastBuffer(buf.buffer, buf.byteOffset, n);
    }

    const dst = Buffer.allocUnsafe(n);

    do {
      const buf = this._buffers[0];
      const offset = dst.length - n;

      if (n >= buf.length) {
        dst.set(this._buffers.shift(), offset);
      } else {
        dst.set(new Uint8Array(buf.buffer, buf.byteOffset, n), offset);
        this._buffers[0] = new FastBuffer(
          buf.buffer,
          buf.byteOffset + n,
          buf.length - n
        );
      }

      n -= buf.length;
    } while (n > 0);

    return dst;
  }

  /**
   * Starts the parsing loop.
   *
   * @param {Function} cb Callback
   * @private
   */
  startLoop(cb) {
    this._loop = true;

    do {
      switch (this._state) {
        case GET_INFO:
          this.getInfo(cb);
          break;
        case GET_PAYLOAD_LENGTH_16:
          this.getPayloadLength16(cb);
          break;
        case GET_PAYLOAD_LENGTH_64:
          this.getPayloadLength64(cb);
          break;
        case GET_MASK:
          this.getMask();
          break;
        case GET_DATA:
          this.getData(cb);
          break;
        case INFLATING:
        case DEFER_EVENT:
          this._loop = false;
          return;
      }
    } while (this._loop);

    if (!this._errored) cb();
  }

  /**
   * Reads the first two bytes of a frame.
   *
   * @param {Function} cb Callback
   * @private
   */
  getInfo(cb) {
    if (this._bufferedBytes < 2) {
      this._loop = false;
      return;
    }

    const buf = this.consume(2);

    if ((buf[0] & 0x30) !== 0x00) {
      const error = this.createError(
        RangeError,
        'RSV2 and RSV3 must be clear',
        true,
        1002,
        'WS_ERR_UNEXPECTED_RSV_2_3'
      );

      cb(error);
      return;
    }

    const compressed = (buf[0] & 0x40) === 0x40;

    if (compressed && !this._extensions[PerMessageDeflate.extensionName]) {
      const error = this.createError(
        RangeError,
        'RSV1 must be clear',
        true,
        1002,
        'WS_ERR_UNEXPECTED_RSV_1'
      );

      cb(error);
      return;
    }

    this._fin = (buf[0] & 0x80) === 0x80;
    this._opcode = buf[0] & 0x0f;
    this._payloadLength = buf[1] & 0x7f;

    if (this._opcode === 0x00) {
      if (compressed) {
        const error = this.createError(
          RangeError,
          'RSV1 must be clear',
          true,
          1002,
          'WS_ERR_UNEXPECTED_RSV_1'
        );

        cb(error);
        return;
      }

      if (!this._fragmented) {
        const error = this.createError(
          RangeError,
          'invalid opcode 0',
          true,
          1002,
          'WS_ERR_INVALID_OPCODE'
        );

        cb(error);
        return;
      }

      this._opcode = this._fragmented;
    } else if (this._opcode === 0x01 || this._opcode === 0x02) {
      if (this._fragmented) {
        const error = this.createError(
          RangeError,
          `invalid opcode ${this._opcode}`,
          true,
          1002,
          'WS_ERR_INVALID_OPCODE'
        );

        cb(error);
        return;
      }

      this._compressed = compressed;
    } else if (this._opcode > 0x07 && this._opcode < 0x0b) {
      if (!this._fin) {
        const error = this.createError(
          RangeError,
          'FIN must be set',
          true,
          1002,
          'WS_ERR_EXPECTED_FIN'
        );

        cb(error);
        return;
      }

      if (compressed) {
        const error = this.createError(
          RangeError,
          'RSV1 must be clear',
          true,
          1002,
          'WS_ERR_UNEXPECTED_RSV_1'
        );

        cb(error);
        return;
      }

      if (
        this._payloadLength > 0x7d ||
        (this._opcode === 0x08 && this._payloadLength === 1)
      ) {
        const error = this.createError(
          RangeError,
          `invalid payload length ${this._payloadLength}`,
          true,
          1002,
          'WS_ERR_INVALID_CONTROL_PAYLOAD_LENGTH'
        );

        cb(error);
        return;
      }
    } else {
      const error = this.createError(
        RangeError,
        `invalid opcode ${this._opcode}`,
        true,
        1002,
        'WS_ERR_INVALID_OPCODE'
      );

      cb(error);
      return;
    }

    if (!this._fin && !this._fragmented) this._fragmented = this._opcode;
    this._masked = (buf[1] & 0x80) === 0x80;

    if (this._isServer) {
      if (!this._masked) {
        const error = this.createError(
          RangeError,
          'MASK must be set',
          true,
          1002,
          'WS_ERR_EXPECTED_MASK'
        );

        cb(error);
        return;
      }
    } else if (this._masked) {
      const error = this.createError(
        RangeError,
        'MASK must be clear',
        true,
        1002,
        'WS_ERR_UNEXPECTED_MASK'
      );

      cb(error);
      return;
    }

    if (this._payloadLength === 126) this._state = GET_PAYLOAD_LENGTH_16;
    else if (this._payloadLength === 127) this._state = GET_PAYLOAD_LENGTH_64;
    else this.haveLength(cb);
  }

  /**
   * Gets extended payload length (7+16).
   *
   * @param {Function} cb Callback
   * @private
   */
  getPayloadLength16(cb) {
    if (this._bufferedBytes < 2) {
      this._loop = false;
      return;
    }

    this._payloadLength = this.consume(2).readUInt16BE(0);
    this.haveLength(cb);
  }

  /**
   * Gets extended payload length (7+64).
   *
   * @param {Function} cb Callback
   * @private
   */
  getPayloadLength64(cb) {
    if (this._bufferedBytes < 8) {
      this._loop = false;
      return;
    }

    const buf = this.consume(8);
    const num = buf.readUInt32BE(0);

    //
    // The maximum safe integer in JavaScript is 2^53 - 1. An error is returned
    // if payload length is greater than this number.
    //
    if (num > Math.pow(2, 53 - 32) - 1) {
      const error = this.createError(
        RangeError,
        'Unsupported WebSocket frame: payload length > 2^53 - 1',
        false,
        1009,
        'WS_ERR_UNSUPPORTED_DATA_PAYLOAD_LENGTH'
      );

      cb(error);
      return;
    }

    this._payloadLength = num * Math.pow(2, 32) + buf.readUInt32BE(4);
    this.haveLength(cb);
  }

  /**
   * Payload length has been read.
   *
   * @param {Function} cb Callback
   * @private
   */
  haveLength(cb) {
    if (this._payloadLength && this._opcode < 0x08) {
      this._totalPayloadLength += this._payloadLength;
      if (this._totalPayloadLength > this._maxPayload && this._maxPayload > 0) {
        const error = this.createError(
          RangeError,
          'Max payload size exceeded',
          false,
          1009,
          'WS_ERR_UNSUPPORTED_MESSAGE_LENGTH'
        );

        cb(error);
        return;
      }
    }

    if (this._masked) this._state = GET_MASK;
    else this._state = GET_DATA;
  }

  /**
   * Reads mask bytes.
   *
   * @private
   */
  getMask() {
    if (this._bufferedBytes < 4) {
      this._loop = false;
      return;
    }

    this._mask = this.consume(4);
    this._state = GET_DATA;
  }

  /**
   * Reads data bytes.
   *
   * @param {Function} cb Callback
   * @private
   */
  getData(cb) {
    let data = EMPTY_BUFFER;

    if (this._payloadLength) {
      if (this._bufferedBytes < this._payloadLength) {
        this._loop = false;
        return;
      }

      data = this.consume(this._payloadLength);

      if (
        this._masked &&
        (this._mask[0] | this._mask[1] | this._mask[2] | this._mask[3]) !== 0
      ) {
        unmask(data, this._mask);
      }
    }

    if (this._opcode > 0x07) {
      this.controlMessage(data, cb);
      return;
    }

    if (this._compressed) {
      this._state = INFLATING;
      this.decompress(data, cb);
      return;
    }

    if (data.length) {
      //
      // This message is not compressed so its length is the sum of the payload
      // length of all fragments.
      //
      this._messageLength = this._totalPayloadLength;
      this._fragments.push(data);
    }

    this.dataMessage(cb);
  }

  /**
   * Decompresses data.
   *
   * @param {Buffer} data Compressed data
   * @param {Function} cb Callback
   * @private
   */
  decompress(data, cb) {
    const perMessageDeflate = this._extensions[PerMessageDeflate.extensionName];

    perMessageDeflate.decompress(data, this._fin, (err, buf) => {
      if (err) return cb(err);

      if (buf.length) {
        this._messageLength += buf.length;
        if (this._messageLength > this._maxPayload && this._maxPayload > 0) {
          const error = this.createError(
            RangeError,
            'Max payload size exceeded',
            false,
            1009,
            'WS_ERR_UNSUPPORTED_MESSAGE_LENGTH'
          );

          cb(error);
          return;
        }

        this._fragments.push(buf);
      }

      this.dataMessage(cb);
      if (this._state === GET_INFO) this.startLoop(cb);
    });
  }

  /**
   * Handles a data message.
   *
   * @param {Function} cb Callback
   * @private
   */
  dataMessage(cb) {
    if (!this._fin) {
      this._state = GET_INFO;
      return;
    }

    const messageLength = this._messageLength;
    const fragments = this._fragments;

    this._totalPayloadLength = 0;
    this._messageLength = 0;
    this._fragmented = 0;
    this._fragments = [];

    if (this._opcode === 2) {
      let data;

      if (this._binaryType === 'nodebuffer') {
        data = concat(fragments, messageLength);
      } else if (this._binaryType === 'arraybuffer') {
        data = toArrayBuffer(concat(fragments, messageLength));
      } else if (this._binaryType === 'blob') {
        data = new Blob(fragments);
      } else {
        data = fragments;
      }

      if (this._allowSynchronousEvents) {
        this.emit('message', data, true);
        this._state = GET_INFO;
      } else {
        this._state = DEFER_EVENT;
        setImmediate(() => {
          this.emit('message', data, true);
          this._state = GET_INFO;
          this.startLoop(cb);
        });
      }
    } else {
      const buf = concat(fragments, messageLength);

      if (!this._skipUTF8Validation && !isValidUTF8(buf)) {
        const error = this.createError(
          Error,
          'invalid UTF-8 sequence',
          true,
          1007,
          'WS_ERR_INVALID_UTF8'
        );

        cb(error);
        return;
      }

      if (this._state === INFLATING || this._allowSynchronousEvents) {
        this.emit('message', buf, false);
        this._state = GET_INFO;
      } else {
        this._state = DEFER_EVENT;
        setImmediate(() => {
          this.emit('message', buf, false);
          this._state = GET_INFO;
          this.startLoop(cb);
        });
      }
    }
  }

  /**
   * Handles a control message.
   *
   * @param {Buffer} data Data to handle
   * @return {(Error|RangeError|undefined)} A possible error
   * @private
   */
  controlMessage(data, cb) {
    if (this._opcode === 0x08) {
      if (data.length === 0) {
        this._loop = false;
        this.emit('conclude', 1005, EMPTY_BUFFER);
        this.end();
      } else {
        const code = data.readUInt16BE(0);

        if (!isValidStatusCode(code)) {
          const error = this.createError(
            RangeError,
            `invalid status code ${code}`,
            true,
            1002,
            'WS_ERR_INVALID_CLOSE_CODE'
          );

          cb(error);
          return;
        }

        const buf = new FastBuffer(
          data.buffer,
          data.byteOffset + 2,
          data.length - 2
        );

        if (!this._skipUTF8Validation && !isValidUTF8(buf)) {
          const error = this.createError(
            Error,
            'invalid UTF-8 sequence',
            true,
            1007,
            'WS_ERR_INVALID_UTF8'
          );

          cb(error);
          return;
        }

        this._loop = false;
        this.emit('conclude', code, buf);
        this.end();
      }

      this._state = GET_INFO;
      return;
    }

    if (this._allowSynchronousEvents) {
      this.emit(this._opcode === 0x09 ? 'ping' : 'pong', data);
      this._state = GET_INFO;
    } else {
      this._state = DEFER_EVENT;
      setImmediate(() => {
        this.emit(this._opcode === 0x09 ? 'ping' : 'pong', data);
        this._state = GET_INFO;
        this.startLoop(cb);
      });
    }
  }

  /**
   * Builds an error object.
   *
   * @param {function(new:Error|RangeError)} ErrorCtor The error constructor
   * @param {String} message The error message
   * @param {Boolean} prefix Specifies whether or not to add a default prefix to
   *     `message`
   * @param {Number} statusCode The status code
   * @param {String} errorCode The exposed error code
   * @return {(Error|RangeError)} The error
   * @private
   */
  createError(ErrorCtor, message, prefix, statusCode, errorCode) {
    this._loop = false;
    this._errored = true;

    const err = new ErrorCtor(
      prefix ? `Invalid WebSocket frame: ${message}` : message
    );

    Error.captureStackTrace(err, this.createError);
    err.code = errorCode;
    err[kStatusCode] = statusCode;
    return err;
  }
}

module.exports = Receiver;


/***/ }),

/***/ 70623:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";
/* eslint no-unused-vars: ["error", { "varsIgnorePattern": "^WebSocket$" }] */


const WebSocket = __webpack_require__(55696);
const { Duplex } = __webpack_require__(27910);

/**
 * Emits the `'close'` event on a stream.
 *
 * @param {Duplex} stream The stream.
 * @private
 */
function emitClose(stream) {
  stream.emit('close');
}

/**
 * The listener of the `'end'` event.
 *
 * @private
 */
function duplexOnEnd() {
  if (!this.destroyed && this._writableState.finished) {
    this.destroy();
  }
}

/**
 * The listener of the `'error'` event.
 *
 * @param {Error} err The error
 * @private
 */
function duplexOnError(err) {
  this.removeListener('error', duplexOnError);
  this.destroy();
  if (this.listenerCount('error') === 0) {
    // Do not suppress the throwing behavior.
    this.emit('error', err);
  }
}

/**
 * Wraps a `WebSocket` in a duplex stream.
 *
 * @param {WebSocket} ws The `WebSocket` to wrap
 * @param {Object} [options] The options for the `Duplex` constructor
 * @return {Duplex} The duplex stream
 * @public
 */
function createWebSocketStream(ws, options) {
  let terminateOnDestroy = true;

  const duplex = new Duplex({
    ...options,
    autoDestroy: false,
    emitClose: false,
    objectMode: false,
    writableObjectMode: false
  });

  ws.on('message', function message(msg, isBinary) {
    const data =
      !isBinary && duplex._readableState.objectMode ? msg.toString() : msg;

    if (!duplex.push(data)) ws.pause();
  });

  ws.once('error', function error(err) {
    if (duplex.destroyed) return;

    // Prevent `ws.terminate()` from being called by `duplex._destroy()`.
    //
    // - If the `'error'` event is emitted before the `'open'` event, then
    //   `ws.terminate()` is a noop as no socket is assigned.
    // - Otherwise, the error is re-emitted by the listener of the `'error'`
    //   event of the `Receiver` object. The listener already closes the
    //   connection by calling `ws.close()`. This allows a close frame to be
    //   sent to the other peer. If `ws.terminate()` is called right after this,
    //   then the close frame might not be sent.
    terminateOnDestroy = false;
    duplex.destroy(err);
  });

  ws.once('close', function close() {
    if (duplex.destroyed) return;

    duplex.push(null);
  });

  duplex._destroy = function (err, callback) {
    if (ws.readyState === ws.CLOSED) {
      callback(err);
      process.nextTick(emitClose, duplex);
      return;
    }

    let called = false;

    ws.once('error', function error(err) {
      called = true;
      callback(err);
    });

    ws.once('close', function close() {
      if (!called) callback(err);
      process.nextTick(emitClose, duplex);
    });

    if (terminateOnDestroy) ws.terminate();
  };

  duplex._final = function (callback) {
    if (ws.readyState === ws.CONNECTING) {
      ws.once('open', function open() {
        duplex._final(callback);
      });
      return;
    }

    // If the value of the `_socket` property is `null` it means that `ws` is a
    // client websocket and the handshake failed. In fact, when this happens, a
    // socket is never assigned to the websocket. Wait for the `'error'` event
    // that will be emitted by the websocket.
    if (ws._socket === null) return;

    if (ws._socket._writableState.finished) {
      callback();
      if (duplex._readableState.endEmitted) duplex.destroy();
    } else {
      ws._socket.once('finish', function finish() {
        // `duplex` is not destroyed here because the `'end'` event will be
        // emitted on `duplex` after this `'finish'` event. The EOF signaling
        // `null` chunk is, in fact, pushed when the websocket emits `'close'`.
        callback();
      });
      ws.close();
    }
  };

  duplex._read = function () {
    if (ws.isPaused) ws.resume();
  };

  duplex._write = function (chunk, encoding, callback) {
    if (ws.readyState === ws.CONNECTING) {
      ws.once('open', function open() {
        duplex._write(chunk, encoding, callback);
      });
      return;
    }

    ws.send(chunk, callback);
  };

  duplex.on('end', duplexOnEnd);
  duplex.on('error', duplexOnError);
  return duplex;
}

module.exports = createWebSocketStream;


/***/ }),

/***/ 80581:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   Y: () => (/* binding */ useDebouncedCallback),
/* harmony export */   d: () => (/* binding */ useDebounce)
/* harmony export */ });
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(88782);
/* harmony import */ var react__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(react__WEBPACK_IMPORTED_MODULE_0__);

/**
 * Hook that debounces a value by the specified delay
 * @param value - The value to debounce
 * @param delay - Delay in milliseconds
 * @returns The debounced value
 */ function useDebounce(value, delay) {
    const [debouncedValue, setDebouncedValue] = (0,react__WEBPACK_IMPORTED_MODULE_0__.useState)(value);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        const timer = setTimeout(()=>{
            setDebouncedValue(value);
        }, delay);
        return ()=>{
            clearTimeout(timer);
        };
    }, [
        value,
        delay
    ]);
    return debouncedValue;
}
/**
 * Hook that returns a debounced callback function
 * @param callback - The callback to debounce
 * @param delay - Delay in milliseconds
 * @returns Debounced callback function
 */ function useDebouncedCallback(callback, delay) {
    const timeoutIdRef = (0,react__WEBPACK_IMPORTED_MODULE_0__.useRef)(null);
    const debouncedCallback = (0,react__WEBPACK_IMPORTED_MODULE_0__.useCallback)((...args)=>{
        if (timeoutIdRef.current) {
            clearTimeout(timeoutIdRef.current);
        }
        timeoutIdRef.current = setTimeout(()=>{
            timeoutIdRef.current = null;
            callback(...args);
        }, delay);
    }, [
        callback,
        delay
    ]);
    process.env.__NEXT_PRIVATE_MINIMIZE_MACRO_FALSE && (0,react__WEBPACK_IMPORTED_MODULE_0__.useEffect)(()=>{
        return ()=>{
            if (timeoutIdRef.current) {
                clearTimeout(timeoutIdRef.current);
            }
        };
    }, []);
    return debouncedCallback;
}


/***/ }),

/***/ 81346:
/***/ ((__unused_webpack_module, __webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony export */ __webpack_require__.d(__webpack_exports__, {
/* harmony export */   $o: () => (/* binding */ containsPaneId),
/* harmony export */   Re: () => (/* binding */ swapPanes),
/* harmony export */   Xt: () => (/* binding */ wouldExceedMaxDepth),
/* harmony export */   Zn: () => (/* binding */ MIN_RATIO),
/* harmony export */   cg: () => (/* binding */ generateSecureId),
/* harmony export */   eH: () => (/* binding */ getAllLeaves),
/* harmony export */   fB: () => (/* binding */ createLeaf),
/* harmony export */   fJ: () => (/* binding */ getAdjacentLeaf),
/* harmony export */   fo: () => (/* binding */ replaceNode),
/* harmony export */   gy: () => (/* binding */ findParentSplit),
/* harmony export */   iZ: () => (/* binding */ findLeaf),
/* harmony export */   mJ: () => (/* binding */ findSplit),
/* harmony export */   un: () => (/* binding */ initialPaneState),
/* harmony export */   v2: () => (/* binding */ findNearestAncestorSplit),
/* harmony export */   xk: () => (/* binding */ generatePaneId),
/* harmony export */   zN: () => (/* binding */ MAX_RATIO)
/* harmony export */ });
/* unused harmony exports MAX_DEPTH, hasVerticalSplit */
const MIN_RATIO = 0.1;
const MAX_RATIO = 0.9;
const MAX_DEPTH = 8;
/**
 * Generate an opaque, sufficiently-unique ID string using crypto.randomUUID()
 * where available, falling back to Math.random() outside secure contexts
 * (e.g. non-HTTPS dev servers). Not cryptographically significant — used for
 * client-side correlation (pane IDs, resync correlation IDs), never for
 * security-sensitive purposes.
 */ function generateSecureId() {
    if (typeof crypto !== "undefined" && crypto.randomUUID) {
        return crypto.randomUUID();
    }
    return Math.random().toString(36).slice(2, 10);
}
/** Generate a unique pane ID */ function generatePaneId() {
    return generateSecureId().slice(0, 8);
}
/** Create a new empty leaf pane */ function createLeaf(id, activeTab = "terminal", viewKind = "session-detail") {
    return {
        type: "leaf",
        id: id ?? generatePaneId(),
        sessionId: null,
        activeTab,
        viewKind
    };
}
/** Return the initial pane state: a vertical split with session-list on the left and session-detail on the right */ function initialPaneState() {
    const listLeaf = createLeaf(undefined, "terminal", "session-list");
    const detailLeaf = createLeaf(undefined, "terminal", "session-detail");
    const splitNode = {
        type: "split",
        id: generatePaneId(),
        direction: "vertical",
        ratio: 0.28,
        first: listLeaf,
        second: detailLeaf
    };
    return {
        root: splitNode,
        focusedPaneId: detailLeaf.id,
        zoomedPaneId: null
    };
}
/**
 * Swap the content (viewKind, sessionId, activeTab) of two leaf panes while keeping their IDs.
 * Returns the original root unchanged if either pane is not found.
 */ function swapPanes(root, paneId, targetPaneId) {
    if (paneId === targetPaneId) return root;
    const leaf1 = findLeaf(root, paneId);
    const leaf2 = findLeaf(root, targetPaneId);
    if (!leaf1 || !leaf2) return root;
    const c1 = {
        viewKind: leaf1.viewKind,
        sessionId: leaf1.sessionId,
        activeTab: leaf1.activeTab
    };
    const c2 = {
        viewKind: leaf2.viewKind,
        sessionId: leaf2.sessionId,
        activeTab: leaf2.activeTab
    };
    let result = replaceNode(root, paneId, {
        ...leaf1,
        ...c2
    });
    result = replaceNode(result, targetPaneId, {
        ...leaf2,
        ...c1
    });
    return result;
}
/** Returns true if the tree contains at least one vertical (side-by-side) split */ function hasVerticalSplit(node) {
    if (node.type === "leaf") return false;
    if (node.direction === "vertical") return true;
    return hasVerticalSplit(node.first) || hasVerticalSplit(node.second);
}
/** Collect all leaf panes from the tree */ function getAllLeaves(root) {
    if (root.type === "leaf") return [
        root
    ];
    return [
        ...getAllLeaves(root.first),
        ...getAllLeaves(root.second)
    ];
}
/** Find a leaf by id, returns null if not found */ function findLeaf(root, id) {
    if (root.type === "leaf") {
        return root.id === id ? root : null;
    }
    return findLeaf(root.first, id) ?? findLeaf(root.second, id);
}
/** Find the parent SplitPane of the given node id */ function findParentSplit(root, id) {
    if (root.type === "leaf") return null;
    if (root.first.id === id || root.second.id === id) return root;
    return findParentSplit(root.first, id) ?? findParentSplit(root.second, id);
}
/** Find a SplitPane by its own id */ function findSplit(root, splitId) {
    if (root.type === "leaf") return null;
    if (root.id === splitId) return root;
    return findSplit(root.first, splitId) ?? findSplit(root.second, splitId);
}
/** Check if a pane id exists somewhere in the subtree */ function containsPaneId(node, id) {
    if (node.type === "leaf") return node.id === id;
    return containsPaneId(node.first, id) || containsPaneId(node.second, id);
}
/** Get the current depth of nesting for a given pane id */ function getDepthOf(root, targetId, depth = 0) {
    if (root.type === "leaf") return root.id === targetId ? depth : -1;
    const d1 = getDepthOf(root.first, targetId, depth + 1);
    if (d1 >= 0) return d1;
    return getDepthOf(root.second, targetId, depth + 1);
}
/** Check whether splitting the given leaf would exceed MAX_DEPTH */ function wouldExceedMaxDepth(root, paneId) {
    const depth = getDepthOf(root, paneId);
    return depth < 0 || depth >= MAX_DEPTH;
}
/**
 * Replace the node with the given targetId in the tree, returning a new tree.
 * If not found, returns the original root unchanged.
 */ function replaceNode(root, targetId, replacement) {
    if (root.id === targetId) return replacement;
    if (root.type === "leaf") return root;
    const newFirst = replaceNode(root.first, targetId, replacement);
    const newSecond = replaceNode(root.second, targetId, replacement);
    if (newFirst === root.first && newSecond === root.second) return root;
    return {
        ...root,
        first: newFirst,
        second: newSecond
    };
}
/**
 * Find the nearest ancestor SplitPane whose direction aligns with the given arrow direction.
 * "ArrowLeft"/"ArrowRight" → look for "vertical" splits
 * "ArrowUp"/"ArrowDown"   → look for "horizontal" splits
 */ function findNearestAncestorSplit(root, paneId, arrowDirection) {
    const neededDirection = arrowDirection === "ArrowLeft" || arrowDirection === "ArrowRight" ? "vertical" : "horizontal";
    function search(node, ancestors) {
        if (node.type === "leaf") {
            if (node.id !== paneId) return null;
            // Walk ancestors from nearest to farthest
            for(let i = ancestors.length - 1; i >= 0; i--){
                if (ancestors[i].direction === neededDirection) return ancestors[i];
            }
            return null;
        }
        const withThis = [
            ...ancestors,
            node
        ];
        return search(node.first, withThis) ?? search(node.second, withThis);
    }
    return search(root, []);
}
/**
 * Get the adjacent leaf in a given direction from the focused pane.
 * Returns null if no adjacent pane exists in that direction.
 */ function getAdjacentLeaf(root, fromId, direction) {
    // Determine the split direction we care about
    const splitDir = direction === "ArrowLeft" || direction === "ArrowRight" ? "vertical" : "horizontal";
    const goToSecond = direction === "ArrowRight" || direction === "ArrowDown";
    function findAdjacentInNode(node) {
        if (node.type === "leaf") return null;
        if (node.direction !== splitDir) {
            return findAdjacentInNode(node.first) ?? findAdjacentInNode(node.second);
        }
        // This split is in the right direction
        const inFirst = containsPaneId(node.first, fromId);
        const inSecond = containsPaneId(node.second, fromId);
        if (inFirst && goToSecond) {
            // Move to the first (leftmost/topmost) leaf of second subtree
            return getFirstLeaf(node.second);
        }
        if (inSecond && !goToSecond) {
            // Move to the last (rightmost/bottommost) leaf of first subtree
            return getLastLeaf(node.first);
        }
        // fromId is in one side but we're going the other direction — recurse
        if (inFirst) return findAdjacentInNode(node.first);
        if (inSecond) return findAdjacentInNode(node.second);
        return null;
    }
    return findAdjacentInNode(root);
}
function getFirstLeaf(node) {
    if (node.type === "leaf") return node;
    return getFirstLeaf(node.first);
}
function getLastLeaf(node) {
    if (node.type === "leaf") return node;
    return getLastLeaf(node.second);
}


/***/ }),

/***/ 82524:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";


const { isUtf8 } = __webpack_require__(79428);

const { hasBlob } = __webpack_require__(20670);

//
// Allowed token characters:
//
// '!', '#', '$', '%', '&', ''', '*', '+', '-',
// '.', 0-9, A-Z, '^', '_', '`', a-z, '|', '~'
//
// tokenChars[32] === 0 // ' '
// tokenChars[33] === 1 // '!'
// tokenChars[34] === 0 // '"'
// ...
//
// prettier-ignore
const tokenChars = [
  0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 0 - 15
  0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // 16 - 31
  0, 1, 0, 1, 1, 1, 1, 1, 0, 0, 1, 1, 0, 1, 1, 0, // 32 - 47
  1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, // 48 - 63
  0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 64 - 79
  1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0, 0, 1, 1, // 80 - 95
  1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, // 96 - 111
  1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 0, 1, 0 // 112 - 127
];

/**
 * Checks if a status code is allowed in a close frame.
 *
 * @param {Number} code The status code
 * @return {Boolean} `true` if the status code is valid, else `false`
 * @public
 */
function isValidStatusCode(code) {
  return (
    (code >= 1000 &&
      code <= 1014 &&
      code !== 1004 &&
      code !== 1005 &&
      code !== 1006) ||
    (code >= 3000 && code <= 4999)
  );
}

/**
 * Checks if a given buffer contains only correct UTF-8.
 * Ported from https://www.cl.cam.ac.uk/%7Emgk25/ucs/utf8_check.c by
 * Markus Kuhn.
 *
 * @param {Buffer} buf The buffer to check
 * @return {Boolean} `true` if `buf` contains only correct UTF-8, else `false`
 * @public
 */
function _isValidUTF8(buf) {
  const len = buf.length;
  let i = 0;

  while (i < len) {
    if ((buf[i] & 0x80) === 0) {
      // 0xxxxxxx
      i++;
    } else if ((buf[i] & 0xe0) === 0xc0) {
      // 110xxxxx 10xxxxxx
      if (
        i + 1 === len ||
        (buf[i + 1] & 0xc0) !== 0x80 ||
        (buf[i] & 0xfe) === 0xc0 // Overlong
      ) {
        return false;
      }

      i += 2;
    } else if ((buf[i] & 0xf0) === 0xe0) {
      // 1110xxxx 10xxxxxx 10xxxxxx
      if (
        i + 2 >= len ||
        (buf[i + 1] & 0xc0) !== 0x80 ||
        (buf[i + 2] & 0xc0) !== 0x80 ||
        (buf[i] === 0xe0 && (buf[i + 1] & 0xe0) === 0x80) || // Overlong
        (buf[i] === 0xed && (buf[i + 1] & 0xe0) === 0xa0) // Surrogate (U+D800 - U+DFFF)
      ) {
        return false;
      }

      i += 3;
    } else if ((buf[i] & 0xf8) === 0xf0) {
      // 11110xxx 10xxxxxx 10xxxxxx 10xxxxxx
      if (
        i + 3 >= len ||
        (buf[i + 1] & 0xc0) !== 0x80 ||
        (buf[i + 2] & 0xc0) !== 0x80 ||
        (buf[i + 3] & 0xc0) !== 0x80 ||
        (buf[i] === 0xf0 && (buf[i + 1] & 0xf0) === 0x80) || // Overlong
        (buf[i] === 0xf4 && buf[i + 1] > 0x8f) ||
        buf[i] > 0xf4 // > U+10FFFF
      ) {
        return false;
      }

      i += 4;
    } else {
      return false;
    }
  }

  return true;
}

/**
 * Determines whether a value is a `Blob`.
 *
 * @param {*} value The value to be tested
 * @return {Boolean} `true` if `value` is a `Blob`, else `false`
 * @private
 */
function isBlob(value) {
  return (
    hasBlob &&
    typeof value === 'object' &&
    typeof value.arrayBuffer === 'function' &&
    typeof value.type === 'string' &&
    typeof value.stream === 'function' &&
    (value[Symbol.toStringTag] === 'Blob' ||
      value[Symbol.toStringTag] === 'File')
  );
}

module.exports = {
  isBlob,
  isValidStatusCode,
  isValidUTF8: _isValidUTF8,
  tokenChars
};

if (isUtf8) {
  module.exports.isValidUTF8 = function (buf) {
    return buf.length < 24 ? _isValidUTF8(buf) : isUtf8(buf);
  };
} /* istanbul ignore else  */ else if (!process.env.WS_NO_UTF_8_VALIDATE) {
  try {
    const isValidUTF8 = __webpack_require__(83610);

    module.exports.isValidUTF8 = function (buf) {
      return buf.length < 32 ? _isValidUTF8(buf) : isValidUTF8(buf);
    };
  } catch (e) {
    // Continue regardless of the error.
  }
}


/***/ }),

/***/ 83610:
/***/ (() => {

/* (ignored) */

/***/ }),

/***/ 94505:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";


const { kForOnEventAttribute, kListener } = __webpack_require__(20670);

const kCode = Symbol('kCode');
const kData = Symbol('kData');
const kError = Symbol('kError');
const kMessage = Symbol('kMessage');
const kReason = Symbol('kReason');
const kTarget = Symbol('kTarget');
const kType = Symbol('kType');
const kWasClean = Symbol('kWasClean');

/**
 * Class representing an event.
 */
class Event {
  /**
   * Create a new `Event`.
   *
   * @param {String} type The name of the event
   * @throws {TypeError} If the `type` argument is not specified
   */
  constructor(type) {
    this[kTarget] = null;
    this[kType] = type;
  }

  /**
   * @type {*}
   */
  get target() {
    return this[kTarget];
  }

  /**
   * @type {String}
   */
  get type() {
    return this[kType];
  }
}

Object.defineProperty(Event.prototype, 'target', { enumerable: true });
Object.defineProperty(Event.prototype, 'type', { enumerable: true });

/**
 * Class representing a close event.
 *
 * @extends Event
 */
class CloseEvent extends Event {
  /**
   * Create a new `CloseEvent`.
   *
   * @param {String} type The name of the event
   * @param {Object} [options] A dictionary object that allows for setting
   *     attributes via object members of the same name
   * @param {Number} [options.code=0] The status code explaining why the
   *     connection was closed
   * @param {String} [options.reason=''] A human-readable string explaining why
   *     the connection was closed
   * @param {Boolean} [options.wasClean=false] Indicates whether or not the
   *     connection was cleanly closed
   */
  constructor(type, options = {}) {
    super(type);

    this[kCode] = options.code === undefined ? 0 : options.code;
    this[kReason] = options.reason === undefined ? '' : options.reason;
    this[kWasClean] = options.wasClean === undefined ? false : options.wasClean;
  }

  /**
   * @type {Number}
   */
  get code() {
    return this[kCode];
  }

  /**
   * @type {String}
   */
  get reason() {
    return this[kReason];
  }

  /**
   * @type {Boolean}
   */
  get wasClean() {
    return this[kWasClean];
  }
}

Object.defineProperty(CloseEvent.prototype, 'code', { enumerable: true });
Object.defineProperty(CloseEvent.prototype, 'reason', { enumerable: true });
Object.defineProperty(CloseEvent.prototype, 'wasClean', { enumerable: true });

/**
 * Class representing an error event.
 *
 * @extends Event
 */
class ErrorEvent extends Event {
  /**
   * Create a new `ErrorEvent`.
   *
   * @param {String} type The name of the event
   * @param {Object} [options] A dictionary object that allows for setting
   *     attributes via object members of the same name
   * @param {*} [options.error=null] The error that generated this event
   * @param {String} [options.message=''] The error message
   */
  constructor(type, options = {}) {
    super(type);

    this[kError] = options.error === undefined ? null : options.error;
    this[kMessage] = options.message === undefined ? '' : options.message;
  }

  /**
   * @type {*}
   */
  get error() {
    return this[kError];
  }

  /**
   * @type {String}
   */
  get message() {
    return this[kMessage];
  }
}

Object.defineProperty(ErrorEvent.prototype, 'error', { enumerable: true });
Object.defineProperty(ErrorEvent.prototype, 'message', { enumerable: true });

/**
 * Class representing a message event.
 *
 * @extends Event
 */
class MessageEvent extends Event {
  /**
   * Create a new `MessageEvent`.
   *
   * @param {String} type The name of the event
   * @param {Object} [options] A dictionary object that allows for setting
   *     attributes via object members of the same name
   * @param {*} [options.data=null] The message content
   */
  constructor(type, options = {}) {
    super(type);

    this[kData] = options.data === undefined ? null : options.data;
  }

  /**
   * @type {*}
   */
  get data() {
    return this[kData];
  }
}

Object.defineProperty(MessageEvent.prototype, 'data', { enumerable: true });

/**
 * This provides methods for emulating the `EventTarget` interface. It's not
 * meant to be used directly.
 *
 * @mixin
 */
const EventTarget = {
  /**
   * Register an event listener.
   *
   * @param {String} type A string representing the event type to listen for
   * @param {(Function|Object)} handler The listener to add
   * @param {Object} [options] An options object specifies characteristics about
   *     the event listener
   * @param {Boolean} [options.once=false] A `Boolean` indicating that the
   *     listener should be invoked at most once after being added. If `true`,
   *     the listener would be automatically removed when invoked.
   * @public
   */
  addEventListener(type, handler, options = {}) {
    for (const listener of this.listeners(type)) {
      if (
        !options[kForOnEventAttribute] &&
        listener[kListener] === handler &&
        !listener[kForOnEventAttribute]
      ) {
        return;
      }
    }

    let wrapper;

    if (type === 'message') {
      wrapper = function onMessage(data, isBinary) {
        const event = new MessageEvent('message', {
          data: isBinary ? data : data.toString()
        });

        event[kTarget] = this;
        callListener(handler, this, event);
      };
    } else if (type === 'close') {
      wrapper = function onClose(code, message) {
        const event = new CloseEvent('close', {
          code,
          reason: message.toString(),
          wasClean: this._closeFrameReceived && this._closeFrameSent
        });

        event[kTarget] = this;
        callListener(handler, this, event);
      };
    } else if (type === 'error') {
      wrapper = function onError(error) {
        const event = new ErrorEvent('error', {
          error,
          message: error.message
        });

        event[kTarget] = this;
        callListener(handler, this, event);
      };
    } else if (type === 'open') {
      wrapper = function onOpen() {
        const event = new Event('open');

        event[kTarget] = this;
        callListener(handler, this, event);
      };
    } else {
      return;
    }

    wrapper[kForOnEventAttribute] = !!options[kForOnEventAttribute];
    wrapper[kListener] = handler;

    if (options.once) {
      this.once(type, wrapper);
    } else {
      this.on(type, wrapper);
    }
  },

  /**
   * Remove an event listener.
   *
   * @param {String} type A string representing the event type to remove
   * @param {(Function|Object)} handler The listener to remove
   * @public
   */
  removeEventListener(type, handler) {
    for (const listener of this.listeners(type)) {
      if (listener[kListener] === handler && !listener[kForOnEventAttribute]) {
        this.removeListener(type, listener);
        break;
      }
    }
  }
};

module.exports = {
  CloseEvent,
  ErrorEvent,
  Event,
  EventTarget,
  MessageEvent
};

/**
 * Call an event listener
 *
 * @param {(Function|Object)} listener The listener to call
 * @param {*} thisArg The value to use as `this`` when calling the listener
 * @param {Event} event The event to pass to the listener
 * @private
 */
function callListener(listener, thisArg, event) {
  if (typeof listener === 'object' && listener.handleEvent) {
    listener.handleEvent.call(listener, event);
  } else {
    listener.call(thisArg, event);
  }
}


/***/ }),

/***/ 94898:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "@keyframes qdlzem0 {\n  0% {\n    opacity: 0;\n    transform: translateX(-50%) translateY(4px);\n  }\n  100% {\n    opacity: 1;\n    transform: translateX(-50%) translateY(0);\n  }\n}\n.qdlzem1 {\n  position: fixed;\n  bottom: 80px;\n  left: 50%;\n  transform: translateX(-50%);\n  z-index: 1085;\n  padding: var(--_19ez2rz2y) var(--_19ez2rz30);\n  background: var(--_19ez2rzy);\n  color: var(--_19ez2rzz);\n  border-radius: var(--_19ez2rz39);\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3g);\n  box-shadow: 0 2px 8px rgba(0,0,0,0.3);\n  pointer-events: none;\n  animation: qdlzem0 150ms ease-out;\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ }),

/***/ 95498:
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

/***/ 95954:
/***/ ((module, __unused_webpack_exports, __webpack_require__) => {

"use strict";


const { EMPTY_BUFFER } = __webpack_require__(20670);

const FastBuffer = Buffer[Symbol.species];

/**
 * Merges an array of buffers into a new buffer.
 *
 * @param {Buffer[]} list The array of buffers to concat
 * @param {Number} totalLength The total length of buffers in the list
 * @return {Buffer} The resulting buffer
 * @public
 */
function concat(list, totalLength) {
  if (list.length === 0) return EMPTY_BUFFER;
  if (list.length === 1) return list[0];

  const target = Buffer.allocUnsafe(totalLength);
  let offset = 0;

  for (let i = 0; i < list.length; i++) {
    const buf = list[i];
    target.set(buf, offset);
    offset += buf.length;
  }

  if (offset < totalLength) {
    return new FastBuffer(target.buffer, target.byteOffset, offset);
  }

  return target;
}

/**
 * Masks a buffer using the given mask.
 *
 * @param {Buffer} source The buffer to mask
 * @param {Buffer} mask The mask to use
 * @param {Buffer} output The buffer where to store the result
 * @param {Number} offset The offset at which to start writing
 * @param {Number} length The number of bytes to mask.
 * @public
 */
function _mask(source, mask, output, offset, length) {
  for (let i = 0; i < length; i++) {
    output[offset + i] = source[i] ^ mask[i & 3];
  }
}

/**
 * Unmasks a buffer using the given mask.
 *
 * @param {Buffer} buffer The buffer to unmask
 * @param {Buffer} mask The mask to use
 * @public
 */
function _unmask(buffer, mask) {
  for (let i = 0; i < buffer.length; i++) {
    buffer[i] ^= mask[i & 3];
  }
}

/**
 * Converts a buffer to an `ArrayBuffer`.
 *
 * @param {Buffer} buf The buffer to convert
 * @return {ArrayBuffer} Converted buffer
 * @public
 */
function toArrayBuffer(buf) {
  if (buf.length === buf.buffer.byteLength) {
    return buf.buffer;
  }

  return buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.length);
}

/**
 * Converts `data` to a `Buffer`.
 *
 * @param {*} data The data to convert
 * @return {Buffer} The buffer
 * @throws {TypeError}
 * @public
 */
function toBuffer(data) {
  toBuffer.readOnly = true;

  if (Buffer.isBuffer(data)) return data;

  let buf;

  if (data instanceof ArrayBuffer) {
    buf = new FastBuffer(data);
  } else if (ArrayBuffer.isView(data)) {
    buf = new FastBuffer(data.buffer, data.byteOffset, data.byteLength);
  } else {
    buf = Buffer.from(data);
    toBuffer.readOnly = false;
  }

  return buf;
}

module.exports = {
  concat,
  mask: _mask,
  toArrayBuffer,
  toBuffer,
  unmask: _unmask
};

/* istanbul ignore else  */
if (!process.env.WS_NO_BUFFER_UTIL) {
  try {
    const bufferUtil = __webpack_require__(21579);

    module.exports.mask = function (source, mask, output, offset, length) {
      if (length < 48) _mask(source, mask, output, offset, length);
      else bufferUtil.mask(source, mask, output, offset, length);
    };

    module.exports.unmask = function (buffer, mask) {
      if (buffer.length < 32) _unmask(buffer, mask);
      else bufferUtil.unmask(buffer, mask);
    };
  } catch (e) {
    // Continue regardless of the error.
  }
}


/***/ }),

/***/ 97986:
/***/ ((module, __unused_webpack___webpack_exports__, __webpack_require__) => {

"use strict";
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__ = __webpack_require__(40934);
/* harmony import */ var _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default = /*#__PURE__*/__webpack_require__.n(_next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0__);
// Imports

var ___CSS_LOADER_EXPORT___ = _next_15_3_2_c47b41e60e61c3b4_node_modules_next_dist_build_webpack_loaders_css_loader_src_runtime_api_js__WEBPACK_IMPORTED_MODULE_0___default()(false);
// Module
___CSS_LOADER_EXPORT___.push([module.id, "@keyframes osx6i82 {\n  0%, 100% {\n    box-shadow: 0 0 0 0 rgba(0,0,0,0);\n  }\n  50% {\n    box-shadow: 0 0 0 3px var(--_19ez2rzx);\n  }\n}\n.osx6i80 {\n  position: relative;\n  display: inline-flex;\n  align-items: center;\n  gap: var(--_19ez2rz2y);\n  padding: var(--_19ez2rz2y) var(--_19ez2rz2z);\n  background: var(--_19ez2rzd);\n  color: var(--_19ez2rz1);\n  border: 1px solid var(--_19ez2rzg);\n  border-radius: var(--_19ez2rz39);\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3g);\n  cursor: default;\n  line-height: 1;\n}\n.osx6i81 {\n  font-size: var(--_19ez2rz3b);\n}\n.osx6i84 {\n  position: absolute;\n  top: 100%;\n  right: 0;\n  margin-top: var(--_19ez2rz2y);\n  padding: var(--_19ez2rz2z);\n  background: var(--_19ez2rzd);\n  color: var(--_19ez2rz1);\n  border: 1px solid var(--_19ez2rzg);\n  border-radius: var(--_19ez2rz37);\n  font-size: var(--_19ez2rz3b);\n  font-weight: var(--_19ez2rz3f);\n  width: 220px;\n  white-space: normal;\n  text-align: left;\n  z-index: 20;\n  box-shadow: 0 2px 8px rgba(0,0,0,0.2);\n}\n@media (prefers-reduced-motion: no-preference) {\n  .osx6i83 {\n    animation: osx6i82 600ms ease-in-out;\n  }\n}\n@media (prefers-reduced-motion: reduce) {\n  .osx6i83 {\n    box-shadow: 0 0 0 3px var(--_19ez2rzx);\n  }\n}", ""]);
// Exports
/* unused harmony default export */ var __WEBPACK_DEFAULT_EXPORT__ = ((/* unused pure expression or super */ null && (___CSS_LOADER_EXPORT___)));


/***/ })

};
;