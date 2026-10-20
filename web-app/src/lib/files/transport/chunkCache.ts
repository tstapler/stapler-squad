import type { FileRef, ReadResult } from "./fileTransport";

const MIB = 1024 * 1024;
export const CHUNK_CACHE_BUDGET_COARSE = 8 * MIB;
export const CHUNK_CACHE_BUDGET_DESKTOP = 64 * MIB;

/** 8 MiB on touch-first devices, 64 MiB elsewhere. */
export function chunkBudgetFor(coarsePointer: boolean): number {
  return coarsePointer ? CHUNK_CACHE_BUDGET_COARSE : CHUNK_CACHE_BUDGET_DESKTOP;
}

interface FileEntry { validator: string; chunks: Map<number, ReadResult> }
interface Slot { file: string; startLine: number; bytes: number }

const sizeOf = (c: ReadResult) => Math.max(c.range.endByte - c.range.startByte, c.text.length);

/**
 * LRU-by-bytes store of line windows per file. A chunk is only valid for the validator it was read under:
 * `put` with a different validator empties the file first and reports it.
 */
export class ChunkCache {
  private readonly files = new Map<string, FileEntry>();
  private readonly lru = new Map<string, Slot>(); // insertion order = recency
  private used = 0;

  constructor(readonly budgetBytes: number) {}

  get bytes(): number {
    return this.used;
  }

  /** The validator the cached chunks of `ref` were read under, if any. */
  validatorOf(ref: FileRef): string | undefined {
    return this.files.get(keyOf(ref))?.validator;
  }

  /** Stores a chunk; returns true when this replaced the chunks of an older validator. */
  put(ref: FileRef, validator: string, chunk: ReadResult): boolean {
    const file = keyOf(ref);
    let entry = this.files.get(file);
    const changed = entry !== undefined && entry.validator !== validator;
    if (changed) this.invalidate(ref);
    if (!entry || changed) {
      entry = { validator, chunks: new Map() };
      this.files.set(file, entry);
    }
    const bytes = sizeOf(chunk);
    if (bytes > this.budgetBytes) return changed;
    this.drop(file, chunk.range.startLine);
    entry.chunks.set(chunk.range.startLine, chunk);
    this.lru.set(slotKey(file, chunk.range.startLine), { file, startLine: chunk.range.startLine, bytes });
    this.used += bytes;
    while (this.used > this.budgetBytes) this.evictOldest();
    return changed;
  }

  /**
   * Whole chunks that together cover lines [first, last] with no gap, as one result; undefined on any gap.
   * A window never starts or ends mid-line (partial last lines are not stitched across).
   */
  get(ref: FileRef, first: number, last: number): ReadResult | undefined {
    const file = keyOf(ref);
    const chunks = this.files.get(file)?.chunks;
    if (!chunks) return undefined;
    const sorted = [...chunks.values()].sort((a, b) => a.range.startLine - b.range.startLine);
    const picked: ReadResult[] = [];
    let next = first;
    for (const c of sorted) {
      if (c.range.endLine < next) continue;
      if (c.range.startLine > next) return undefined;
      picked.push(c);
      if (c.range.endLine >= last || c.range.eof) {
        for (const p of picked) this.touch(file, p.range.startLine);
        return picked.length === 1 ? c : stitch(picked);
      }
      if (c.partialLastLine) return undefined;
      next = c.range.endLine + 1;
    }
    return undefined;
  }

  invalidate(ref: FileRef): void {
    const file = keyOf(ref);
    const entry = this.files.get(file);
    if (!entry) return;
    for (const startLine of [...entry.chunks.keys()]) this.drop(file, startLine);
    this.files.delete(file);
  }

  private touch(file: string, startLine: number): void {
    const k = slotKey(file, startLine);
    const slot = this.lru.get(k);
    if (!slot) return;
    this.lru.delete(k);
    this.lru.set(k, slot);
  }

  private drop(file: string, startLine: number): void {
    const k = slotKey(file, startLine);
    const slot = this.lru.get(k);
    if (!slot) return;
    this.lru.delete(k);
    this.used -= slot.bytes;
    this.files.get(file)?.chunks.delete(startLine);
  }

  private evictOldest(): void {
    const [, slot] = this.lru.entries().next().value as [string, Slot];
    this.drop(slot.file, slot.startLine);
  }
}

const keyOf = (ref: FileRef) => `${ref.sessionId}\0${ref.path}`;
const slotKey = (file: string, startLine: number) => `${file}\0${startLine}`;

function stitch(chunks: readonly ReadResult[]): ReadResult {
  const first = chunks[0];
  const last = chunks[chunks.length - 1];
  return {
    ...last,
    text: chunks.map((c) => c.text).join(""),
    range: { startLine: first.range.startLine, endLine: last.range.endLine, startByte: first.range.startByte, endByte: last.range.endByte, bof: first.range.bof, eof: last.range.eof },
    chunkCrc32c: undefined,
  };
}
