/**
 * Story 5.3.1a: the shared ADR-001 conformance vectors (also used by the Go codec
 * test) and the shared allowlist fixture, plus the TS codec cases.
 */

import vectors from "./__fixtures__/unary-mux-vectors.json";
import allowlist from "./__fixtures__/ws-unary-allowlist.json";
import {
  FRAME_TYPE,
  MalformedFrameError,
  decodeFrame,
  encodeFrame,
  setFrameId,
  type MuxFrame,
} from "./unary-mux-frame";
import { defined } from "@/lib/utils/defined";

function toHex(b: Uint8Array): string {
  return Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
}

type VectorFrame = (typeof vectors.frames)[number];

/** Builds the codec's frame shape from a vector's JSON `fields`. */
function frameFromVector(f: VectorFrame): MuxFrame {
  const fields = f.fields as Record<string, unknown>;
  const body = typeof fields.bodyHex === "string" ? fromHex(fields.bodyHex) : new Uint8Array(0);
  switch (f.type) {
    case "REQUEST":
      return { type: "REQUEST", id: fields.id as number, method: fields.method as string, meta: fields.meta as never, body };
    case "RESPONSE":
      return { type: "RESPONSE", id: fields.id as number, totalLen: fields.totalLen as number, fin: fields.fin === 1, meta: fields.meta as never, body };
    case "CHUNK":
      return { type: "CHUNK", id: fields.id as number, fin: fields.fin === 1, body };
    case "ERROR":
      return { type: "ERROR", id: fields.id as number, json: fields.json as never };
    default:
      return { type: f.type as "CANCEL" | "PING" | "PONG", id: fields.id as number };
  }
}

function fromHex(hex: string): Uint8Array {
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  return out;
}

describe("unary-mux vectors", () => {
  it("should carry the type byte and request id of every frame", () => {
    expect(vectors.maxFrameBytes).toBe(262144);
    expect(vectors.frames.length).toBeGreaterThan(0);
    const types = vectors.types as Record<string, number>;
    for (const f of vectors.frames) {
      const raw = fromHex(f.hex);
      expect(raw.length).toBeGreaterThanOrEqual(5);
      expect(raw[0]).toBe(types[f.type]);
      const id = new DataView(raw.buffer).getUint32(1, false);
      expect(id).toBe((f.fields as { id: number }).id);
    }
  });

  it("should reference only known frames in reassembly cases", () => {
    const names = new Set(vectors.frames.map((f) => f.name));
    for (const r of vectors.reassembly) {
      expect(fromHex(r.payloadHex).length).toBe(r.totalLen);
      for (const n of r.frames) expect(names.has(n)).toBe(true);
    }
  });

  it("should mark every invalid vector malformed", () => {
    for (const iv of vectors.invalid) {
      expect(iv.expect).toBe("malformed");
      expect(fromHex(iv.hex).length).toBeGreaterThan(0);
    }
  });
});

describe("ws-unary-allowlist fixture", () => {
  it("should load with the agreed shape", () => {
    expect(allowlist.wsUnaryAllowed).toHaveLength(9);
    expect(allowlist.clientHeaders).toEqual(["Connect-Protocol-Version"]);
    expect(allowlist.maxInFlight).toBe(6);
  });
});

describe("unary-mux frame codec", () => {
  it.each(vectors.frames.map((f) => [f.name, f] as const))(
    "encode_should_MatchVectorBytes_When_%s",
    (_name, f) => {
      expect(toHex(encodeFrame(frameFromVector(f)))).toBe(f.hex);
    }
  );

  it.each(vectors.frames.map((f) => [f.name, f] as const))(
    "decode_should_RoundTripVectorFields_When_%s",
    (_name, f) => {
      expect(decodeFrame(fromHex(f.hex))).toEqual(frameFromVector(f));
    }
  );

  it.each(vectors.invalid.map((iv) => [iv.name, iv] as const))(
    "decode_should_ThrowMalformed_When_%s",
    (_name, iv) => {
      expect(() => decodeFrame(fromHex(iv.hex))).toThrow(MalformedFrameError);
    }
  );

  it("reassembly_should_ConcatenateChunksToVectorPayload_When_ResponseFin0ThenChunks", () => {
    for (const r of vectors.reassembly) {
      const parts = r.frames.map((n) => {
        const f = defined(vectors.frames.find((x) => x.name === n));
        return decodeFrame(fromHex(f.hex));
      });
      let total = 0;
      const out: number[] = [];
      for (const p of parts) {
        if (p.type === "RESPONSE" || p.type === "CHUNK") {
          out.push(...p.body);
          total += p.body.length;
        }
      }
      expect(total).toBe(r.totalLen);
      expect(toHex(Uint8Array.from(out))).toBe(r.payloadHex);
    }
  });

  it("setFrameId_should_PatchIdInPlace", () => {
    const raw = encodeFrame({ type: "CANCEL", id: 0 });
    setFrameId(raw, 0xdeadbeef);
    expect(decodeFrame(raw)).toEqual({ type: "CANCEL", id: 0xdeadbeef });
  });

  it("types_should_MatchSharedVectorTypeBytes", () => {
    expect(FRAME_TYPE).toEqual(vectors.types);
  });
});
