import vectors from "./__fixtures__/terminal-mux-vectors.json";
import {
  decodeMuxFrame,
  encodeClose,
  encodeConfig,
  encodeCredit,
  encodeData,
  encodeOpen,
  MalformedMuxFrameError,
} from "./terminal-mux-frame";

const hex = (b: Uint8Array) => Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
const fromHex = (h: string) => new Uint8Array((h.match(/../g) ?? []).map((x) => parseInt(x, 16)));

type Vec = { name: string; hex: string; frame: Record<string, unknown> };

describe("terminal-mux-frame", () => {
  it.each(vectors as Vec[])("matches shared vector $name", ({ hex: h, frame }) => {
    const decoded = decodeMuxFrame(fromHex(h)) as Record<string, unknown>;
    expect(decoded.type).toBe(frame.type);
    expect(decoded.id).toBe(frame.id);
    if (frame.type === "DATA") {
      expect(decoded.kind).toBe(frame.kind);
      expect(decoded.fin).toBe(frame.fin);
      expect(hex(decoded.payload as Uint8Array)).toBe(frame.payloadHex);
      expect(hex(encodeData(frame.id as number, frame.kind as number, frame.fin as boolean, decoded.payload as Uint8Array))).toBe(h);
    }
    if (frame.type === "CREDIT") {
      expect(decoded.credit).toBe(frame.credit);
      expect(hex(encodeCredit(frame.id as number, frame.credit as number))).toBe(h);
    }
    if (frame.type === "CLOSE") {
      expect(decoded.code).toBe(frame.code);
      expect(decoded.reason).toBe(frame.reason);
      expect(hex(encodeClose(frame.id as number, frame.code as number, frame.reason as string))).toBe(h);
    }
    if (frame.type === "OPEN") expect(hex(encodeOpen(frame.id as number))).toBe(h);
  });

  it("round-trips HELLO config", () => {
    const cfg = { poolSize: 3, window: 262144, chunk: 16384, maxStreams: 64 };
    const f = decodeMuxFrame(encodeConfig("HELLO", cfg));
    expect(f).toEqual({ type: "HELLO", id: 0, config: cfg });
  });

  it.each([
    ["empty", new Uint8Array()],
    ["short header", new Uint8Array([3, 0, 0])],
    ["unknown type", new Uint8Array([99, 0, 0, 0, 1])],
    ["data without kind", new Uint8Array([3, 0, 0, 0, 1])],
    ["data bad kind", new Uint8Array([3, 0, 0, 0, 1, 9, 0])],
    ["credit short", new Uint8Array([4, 0, 0, 0, 1, 0])],
    ["close short", new Uint8Array([5, 0, 0, 0, 1, 0])],
    ["hello bad json", new Uint8Array([1, 0, 0, 0, 0, 123])],
  ])("rejects malformed frame: %s", (_n, bytes) => {
    expect(() => decodeMuxFrame(bytes)).toThrow(MalformedMuxFrameError);
  });
});
