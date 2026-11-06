// Task 1.6.7b: recorded, never gated. Prints one JSON line per measurement for ADR-036 "Spike outcomes"; the assertions
// below only check that the harness measured something real (jest/jsdom on the dev host, not the phone).
import { ChunkCache, CHUNK_CACHE_BUDGET_COARSE } from "../transport/chunkCache";
import { crc32c } from "../transport/crc32c";
import type { FileRef, ReadResult } from "../transport/fileTransport";

const KIB = 1024;
const MIB = KIB * KIB;
const WINDOW_BYTES = 128 * KIB;
const WINDOW_LINES = 400;
const REF: FileRef = { sessionId: "bench", path: "big.log" };
const FILE_BYTES = 50 * MIB;

function windowAt(i: number): ReadResult {
  const startLine = i * WINDOW_LINES + 1;
  const text = ("L" + String(startLine).padStart(7, "0") + "x".repeat(318) + "\n").repeat(WINDOW_LINES).slice(0, WINDOW_BYTES);
  return {
    text, token: "-", size: FILE_BYTES, lineEnding: "lf", hasBom: false, readOnly: "none", isBinary: false, truncated: true,
    range: { startLine, endLine: startLine + WINDOW_LINES - 1, startByte: i * WINDOW_BYTES, endByte: (i + 1) * WINDOW_BYTES, bof: i === 0, eof: false },
  };
}

const ms = (since: number) => Math.round((performance.now() - since) * 1000) / 1000;
const record = (name: string, data: Record<string, number | string>) => console.log(`WINDOW_BENCH ${JSON.stringify({ name, ...data })}`);

describe("window benchmarks (recorded, not gated)", () => {
  it("windowBench_should_RecordWindowSwapTimeAndChunkCacheMemory_OnPhoneBudget", () => {
    const cache = new ChunkCache(CHUNK_CACHE_BUDGET_COARSE);
    const windows = FILE_BYTES / WINDOW_BYTES; // 400 windows walk through a 50 MB file; 64 of them fit the 8 MiB budget
    const putStart = performance.now();
    for (let i = 0; i < windows; i++) cache.put(REF, "v1", windowAt(i));
    const putMs = ms(putStart);

    const getStart = performance.now();
    const hit = cache.get(REF, (windows - 1) * WINDOW_LINES + 1, (windows - 1) * WINDOW_LINES + WINDOW_LINES);
    const hitMs = ms(getStart);
    const crcInput = new TextEncoder().encode(windowAt(0).text);
    const crcStart = performance.now();
    for (let i = 0; i < 20; i++) crc32c(crcInput);
    const crcMs = ms(crcStart) / 20;

    record("chunkCache8MiB", {
      windows, putTotalMs: putMs, putPerWindowMs: putMs / windows, cachedHitMs: hitMs, crc32cPer128KiBMs: crcMs, cacheBytes: cache.bytes,
    });
    expect(hit?.text.length).toBeGreaterThan(0);
    expect(cache.bytes).toBeLessThanOrEqual(CHUNK_CACHE_BUDGET_COARSE);
  });

  it("windowBench_should_RecordPayloadOfAWindowAgainstAWholeFileRead_OfTheSameFile", () => {
    const w = windowAt(0).text;
    const encoder = new TextEncoder();
    const windowBytes = encoder.encode(w).length;
    record("payload", { windowBytes, wholeFileBytes: FILE_BYTES, ratio: Math.round((windowBytes / FILE_BYTES) * 100000) / 100000 });
    expect(windowBytes).toBeLessThan(FILE_BYTES / 100);
  });
});
