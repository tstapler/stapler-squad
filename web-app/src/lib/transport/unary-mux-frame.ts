/**
 * Codec for the unary-over-WebSocket mux frames of ADR-001 (big-endian):
 * type u8 | request id u32 | type-specific body. Mirrors server/services/ws_unary_frame.go
 * and is checked against the shared vectors in __fixtures__/unary-mux-vectors.json.
 */

export const FRAME_TYPE = {
  REQUEST: 1,
  RESPONSE: 2,
  ERROR: 3,
  CANCEL: 4,
  PING: 5,
  PONG: 6,
  CHUNK: 7,
} as const;

export type FrameTypeName = keyof typeof FRAME_TYPE;

/** Whole encoded frame cap (type + id + method + meta + body), ADR-001. */
export const MAX_FRAME_BYTES = 262144;

export interface RequestMeta {
  headers: Record<string, string>;
  timeoutMs: number;
}

export interface ResponseMeta {
  headers: Record<string, string>;
  trailers: Record<string, string>;
}

export interface ErrorJson {
  code: string;
  message?: string;
  details?: { marker?: string } & Record<string, unknown>;
  headers?: Record<string, string>;
  trailers?: Record<string, string>;
}

export type MuxFrame =
  | { type: "REQUEST"; id: number; method: string; meta: RequestMeta; body: Uint8Array }
  | { type: "RESPONSE"; id: number; totalLen: number; fin: boolean; meta: ResponseMeta; body: Uint8Array }
  | { type: "CHUNK"; id: number; fin: boolean; body: Uint8Array }
  | { type: "ERROR"; id: number; json: ErrorJson }
  | { type: "CANCEL" | "PING" | "PONG"; id: number };

export class MalformedFrameError extends Error {
  constructor(reason: string) {
    super(`malformed unary-mux frame: ${reason}`);
    this.name = "MalformedFrameError";
  }
}

const textEncoder = new TextEncoder();
const textDecoder = new TextDecoder("utf-8", { fatal: true });

function concat(parts: Uint8Array[]): Uint8Array {
  let n = 0;
  for (const p of parts) n += p.length;
  const out = new Uint8Array(n);
  let off = 0;
  for (const p of parts) {
    out.set(p, off);
    off += p.length;
  }
  return out;
}

function header(type: FrameTypeName, id: number): Uint8Array {
  const h = new Uint8Array(5);
  h[0] = FRAME_TYPE[type];
  new DataView(h.buffer).setUint32(1, id, false);
  return h;
}

function u16(n: number): Uint8Array {
  const b = new Uint8Array(2);
  new DataView(b.buffer).setUint16(0, n, false);
  return b;
}

function u32(n: number): Uint8Array {
  const b = new Uint8Array(4);
  new DataView(b.buffer).setUint32(0, n, false);
  return b;
}

export function encodeFrame(frame: MuxFrame): Uint8Array {
  const head = header(frame.type, frame.id);
  switch (frame.type) {
    case "REQUEST": {
      const method = textEncoder.encode(frame.method);
      const meta = textEncoder.encode(JSON.stringify(frame.meta));
      return concat([head, u16(method.length), method, u16(meta.length), meta, frame.body]);
    }
    case "RESPONSE": {
      const meta = textEncoder.encode(JSON.stringify(frame.meta));
      return concat([head, u32(frame.totalLen), Uint8Array.of(frame.fin ? 1 : 0), u16(meta.length), meta, frame.body]);
    }
    case "CHUNK":
      return concat([head, Uint8Array.of(frame.fin ? 1 : 0), frame.body]);
    case "ERROR":
      return concat([head, textEncoder.encode(JSON.stringify(frame.json))]);
    default:
      return head;
  }
}

/** Rewrites the request id (bytes 1-4) of an already-encoded frame. */
export function setFrameId(raw: Uint8Array, id: number): void {
  new DataView(raw.buffer, raw.byteOffset, raw.byteLength).setUint32(1, id, false);
}

function parseJson<T>(bytes: Uint8Array, what: string): T {
  try {
    return JSON.parse(textDecoder.decode(bytes)) as T;
  } catch {
    throw new MalformedFrameError(`${what} is not valid JSON`);
  }
}

/** Throws MalformedFrameError for any frame that violates ADR-001. */
export function decodeFrame(raw: Uint8Array): MuxFrame {
  if (raw.length < 5) throw new MalformedFrameError("shorter than the 5-byte header");
  const view = new DataView(raw.buffer, raw.byteOffset, raw.byteLength);
  const id = view.getUint32(1, false);
  const body = raw.subarray(5);
  switch (raw[0]) {
    case FRAME_TYPE.REQUEST: {
      if (body.length < 2) throw new MalformedFrameError("REQUEST missing method length");
      const methodLen = view.getUint16(5, false);
      let off = 7;
      if (off + methodLen + 2 > raw.length) throw new MalformedFrameError("REQUEST method length overruns frame");
      const method = textDecoder.decode(raw.subarray(off, off + methodLen));
      off += methodLen;
      const metaLen = view.getUint16(off, false);
      off += 2;
      if (off + metaLen > raw.length) throw new MalformedFrameError("REQUEST meta length overruns frame");
      const meta = parseJson<RequestMeta>(raw.subarray(off, off + metaLen), "REQUEST meta");
      return { type: "REQUEST", id, method, meta, body: raw.subarray(off + metaLen) };
    }
    case FRAME_TYPE.RESPONSE: {
      if (body.length < 7) throw new MalformedFrameError("RESPONSE head too short");
      const totalLen = view.getUint32(5, false);
      const fin = raw[9] === 1;
      const metaLen = view.getUint16(10, false);
      if (12 + metaLen > raw.length) throw new MalformedFrameError("RESPONSE meta length overruns frame");
      const meta = parseJson<ResponseMeta>(raw.subarray(12, 12 + metaLen), "RESPONSE meta");
      const payload = raw.subarray(12 + metaLen);
      if (fin && payload.length !== totalLen) throw new MalformedFrameError("RESPONSE fin=1 payload length differs from totalLen");
      if (payload.length > totalLen) throw new MalformedFrameError("RESPONSE payload exceeds totalLen");
      return { type: "RESPONSE", id, totalLen, fin, meta, body: payload };
    }
    case FRAME_TYPE.CHUNK:
      if (body.length < 1) throw new MalformedFrameError("CHUNK missing fin byte");
      return { type: "CHUNK", id, fin: body[0] === 1, body: body.subarray(1) };
    case FRAME_TYPE.ERROR:
      return { type: "ERROR", id, json: parseJson<ErrorJson>(body, "ERROR body") };
    case FRAME_TYPE.CANCEL:
      return { type: "CANCEL", id };
    case FRAME_TYPE.PING:
      return { type: "PING", id };
    case FRAME_TYPE.PONG:
      return { type: "PONG", id };
    default:
      throw new MalformedFrameError(`unknown type ${raw[0]}`);
  }
}
