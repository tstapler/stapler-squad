/**
 * Harness for Story 5.3.1a: loads the shared ADR-001 conformance vectors and the
 * shared allowlist fixture. Codec cases are added by Task 5.3.1a; until the codec
 * exists this file only checks that the vectors are self-consistent.
 */

import vectors from "./__fixtures__/unary-mux-vectors.json";
import allowlist from "./__fixtures__/ws-unary-allowlist.json";

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
