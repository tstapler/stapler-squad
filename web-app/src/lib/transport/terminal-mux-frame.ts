/**
 * Codec for the terminal mux frames of ADR-033 (big-endian): type u8 | stream id u32 | body.
 * Mirrors server/services/ws_terminal_mux_frame.go; checked against the shared vectors in
 * __fixtures__/terminal-mux-vectors.json.
 */

export const MUX_FRAME = {
  HELLO: 1,
  OPEN: 2,
  DATA: 3,
  CREDIT: 4,
  CLOSE: 5,
  PING: 6,
  PONG: 7,
  CONFIG: 8,
} as const;

export const MUX_PATH = "/api/ws/terminal-mux";
export const MUX_KIND_TEXT = 1;
export const MUX_KIND_BINARY = 2;
const HEADER_LEN = 5;

export interface MuxConfig {
  poolSize: number;
  window: number;
  chunk: number;
  maxStreams: number;
}

export type MuxFrame =
  | { type: "HELLO" | "CONFIG"; id: number; config: MuxConfig }
  | { type: "DATA"; id: number; kind: number; fin: boolean; payload: Uint8Array }
  | { type: "CREDIT"; id: number; credit: number }
  | { type: "CLOSE"; id: number; code: number; reason: string }
  | { type: "OPEN" | "PING" | "PONG"; id: number };

export class MalformedMuxFrameError extends Error {
  constructor(reason: string) {
    super(`malformed terminal-mux frame: ${reason}`);
    this.name = "MalformedMuxFrameError";
  }
}

const encoder = new TextEncoder();
const decoder = new TextDecoder("utf-8");

function header(type: number, id: number, bodyLen: number): { buf: Uint8Array; view: DataView } {
  const buf = new Uint8Array(HEADER_LEN + bodyLen);
  const view = new DataView(buf.buffer);
  buf[0] = type;
  view.setUint32(1, id >>> 0);
  return { buf, view };
}

export function encodeOpen(id: number): Uint8Array {
  return header(MUX_FRAME.OPEN, id, 0).buf;
}

export function encodePing(): Uint8Array {
  return header(MUX_FRAME.PING, 0, 0).buf;
}

export function encodePong(): Uint8Array {
  return header(MUX_FRAME.PONG, 0, 0).buf;
}

export function encodeData(id: number, kind: number, fin: boolean, payload: Uint8Array): Uint8Array {
  const { buf } = header(MUX_FRAME.DATA, id, 2 + payload.length);
  buf[HEADER_LEN] = kind;
  buf[HEADER_LEN + 1] = fin ? 1 : 0;
  buf.set(payload, HEADER_LEN + 2);
  return buf;
}

export function encodeCredit(id: number, credit: number): Uint8Array {
  const { buf, view } = header(MUX_FRAME.CREDIT, id, 4);
  view.setUint32(HEADER_LEN, credit >>> 0);
  return buf;
}

export function encodeClose(id: number, code: number, reason = ""): Uint8Array {
  const r = encoder.encode(reason);
  const { buf, view } = header(MUX_FRAME.CLOSE, id, 2 + r.length);
  view.setUint16(HEADER_LEN, code);
  buf.set(r, HEADER_LEN + 2);
  return buf;
}

export function encodeConfig(type: "HELLO" | "CONFIG", config: MuxConfig): Uint8Array {
  const body = encoder.encode(JSON.stringify(config));
  const { buf } = header(MUX_FRAME[type], 0, body.length);
  buf.set(body, HEADER_LEN);
  return buf;
}

function decodeConfig(type: "HELLO" | "CONFIG", id: number, body: Uint8Array): MuxFrame {
  let parsed: Partial<MuxConfig>;
  try {
    parsed = JSON.parse(decoder.decode(body)) as Partial<MuxConfig>;
  } catch {
    throw new MalformedMuxFrameError("config json");
  }
  const num = (v: unknown): number => (typeof v === "number" && Number.isFinite(v) ? v : 0);
  return {
    type,
    id,
    config: { poolSize: num(parsed.poolSize), window: num(parsed.window), chunk: num(parsed.chunk), maxStreams: num(parsed.maxStreams) },
  };
}

export function decodeMuxFrame(msg: Uint8Array): MuxFrame {
  if (msg.length < HEADER_LEN) throw new MalformedMuxFrameError(`${msg.length} bytes, need header`);
  const view = new DataView(msg.buffer, msg.byteOffset, msg.byteLength);
  const id = view.getUint32(1);
  const body = msg.subarray(HEADER_LEN);
  switch (msg[0]) {
    case MUX_FRAME.DATA:
      if (body.length < 2 || (body[0] !== MUX_KIND_TEXT && body[0] !== MUX_KIND_BINARY)) throw new MalformedMuxFrameError("bad DATA header");
      return { type: "DATA", id, kind: body[0], fin: body[1] !== 0, payload: body.subarray(2) };
    case MUX_FRAME.CREDIT:
      if (body.length !== 4) throw new MalformedMuxFrameError("CREDIT body");
      return { type: "CREDIT", id, credit: new DataView(body.buffer, body.byteOffset, 4).getUint32(0) };
    case MUX_FRAME.CLOSE:
      if (body.length < 2) throw new MalformedMuxFrameError("CLOSE body");
      return { type: "CLOSE", id, code: new DataView(body.buffer, body.byteOffset, 2).getUint16(0), reason: decoder.decode(body.subarray(2)) };
    case MUX_FRAME.HELLO:
      return decodeConfig("HELLO", id, body);
    case MUX_FRAME.CONFIG:
      return decodeConfig("CONFIG", id, body);
    case MUX_FRAME.OPEN:
      return { type: "OPEN", id };
    case MUX_FRAME.PING:
      return { type: "PING", id };
    case MUX_FRAME.PONG:
      return { type: "PONG", id };
    default:
      throw new MalformedMuxFrameError(`unknown type ${msg[0]}`);
  }
}
