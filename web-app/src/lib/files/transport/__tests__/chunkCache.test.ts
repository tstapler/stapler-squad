import {
  CHUNK_CACHE_BUDGET_COARSE, CHUNK_CACHE_BUDGET_DESKTOP, ChunkCache, chunkBudgetFor,
} from "../chunkCache";
import type { FileRef, ReadResult } from "../fileTransport";

const KIB = 1024;
const REF: FileRef = { sessionId: "s1", path: "big.log" };

/** A window of lines [start, end] holding `bytes` raw bytes at byte offset `at`. */
function chunk(start: number, end: number, over: { bytes?: number; at?: number; eof?: boolean; partial?: boolean; text?: string } = {}): ReadResult {
  const bytes = over.bytes ?? 100;
  const at = over.at ?? (start - 1) * 10;
  return {
    text: over.text ?? `L${start}-${end};`, token: "-", size: 1e9, lineEnding: "lf", hasBom: false, readOnly: "none", isBinary: false, truncated: true,
    range: { startLine: start, endLine: end, startByte: at, endByte: at + bytes, bof: start === 1, eof: over.eof ?? false },
    partialLastLine: over.partial,
  };
}

describe("ChunkCache", () => {
  it("chunkCache_should_HoldAtMostBudgetBytes_And_StitchWindowsFromChunks_And_EvictLru", () => {
    // Budgets: 8 MiB on touch devices, 64 MiB on desktop, by raw byte accounting over 200 windows of 128 KiB.
    expect(chunkBudgetFor(true)).toBe(8 * 1024 * KIB);
    expect(chunkBudgetFor(false)).toBe(64 * 1024 * KIB);
    for (const [budget, expected] of [[CHUNK_CACHE_BUDGET_COARSE, 64], [CHUNK_CACHE_BUDGET_DESKTOP, 200]] as const) {
      const cache = new ChunkCache(budget);
      for (let i = 0; i < 200; i++) {
        cache.put(REF, "v1", chunk(i * 400 + 1, i * 400 + 400, { bytes: 128 * KIB, text: "x" }));
        expect(cache.bytes).toBeLessThanOrEqual(budget);
      }
      expect(cache.bytes).toBe(expected * 128 * KIB);
      expect(cache.get(REF, 199 * 400 + 1, 199 * 400 + 400)).toBeDefined(); // newest kept
      expect(cache.get(REF, 1, 400) !== undefined).toBe(expected === 200); // oldest evicted only when over budget
    }

    // Adjacent chunks stitch into one window with the first start and last end; a gap or a partial line does not.
    const cache = new ChunkCache(1 * 1024 * KIB);
    cache.put(REF, "v1", chunk(1, 10, { text: "a", at: 0, bytes: 10 }));
    cache.put(REF, "v1", chunk(11, 20, { text: "b", at: 10, bytes: 10 }));
    cache.put(REF, "v1", chunk(31, 40, { text: "d", at: 30, bytes: 10 }));
    const stitched = cache.get(REF, 5, 15);
    expect(stitched).toMatchObject({ text: "ab", range: { startLine: 1, endLine: 20, startByte: 0, endByte: 20, bof: true, eof: false } });
    expect(cache.get(REF, 15, 35)).toBeUndefined(); // lines 21-30 missing
    cache.put(REF, "v1", chunk(21, 30, { text: "c", at: 20, bytes: 10, partial: true }));
    expect(cache.get(REF, 15, 35)).toBeUndefined(); // a partial last line is never stitched across
    cache.put(REF, "v1", chunk(21, 30, { text: "c", at: 20, bytes: 10 }));
    expect(cache.get(REF, 15, 35)?.text).toBe("bcd");
    cache.put(REF, "v1", chunk(41, 45, { text: "e", at: 40, bytes: 5, eof: true }));
    expect(cache.get(REF, 42, 99)?.text).toBe("e"); // a window past EOF is covered by the EOF chunk

    // LRU: reading a chunk keeps it; the least recently used one goes first.
    const lru = new ChunkCache(250);
    lru.put(REF, "v1", chunk(1, 10, { bytes: 100 }));
    lru.put(REF, "v1", chunk(11, 20, { bytes: 100 }));
    expect(lru.get(REF, 1, 10)).toBeDefined();
    lru.put(REF, "v1", chunk(21, 30, { bytes: 100 }));
    expect(lru.get(REF, 11, 20)).toBeUndefined();
    expect(lru.get(REF, 1, 10)).toBeDefined();
    expect(lru.bytes).toBe(200);
    lru.put(REF, "v1", chunk(31, 40, { bytes: 300 })); // larger than the budget: not stored, nothing evicted for it
    expect(lru.bytes).toBe(200);
  });

  it("chunkCache_should_KeyChunksByValidator_And_EmptyAFileOnChange_WithoutTouchingOthers", () => {
    const cache = new ChunkCache(1024 * KIB);
    const other: FileRef = { sessionId: "s2", path: "big.log" };
    cache.put(REF, "v1", chunk(1, 10));
    cache.put(other, "v9", chunk(1, 10));
    expect(cache.validatorOf(REF)).toBe("v1");
    expect(cache.put(REF, "v1", chunk(11, 20))).toBe(false);
    expect(cache.put(REF, "v2", chunk(21, 30))).toBe(true);
    expect(cache.get(REF, 1, 10)).toBeUndefined();
    expect(cache.get(REF, 21, 30)).toBeDefined();
    expect(cache.validatorOf(other)).toBe("v9");
    expect(cache.get(other, 1, 10)).toBeDefined();
    cache.invalidate(REF);
    expect(cache.bytes).toBe(100);
    expect(cache.validatorOf(REF)).toBeUndefined();
  });
});
