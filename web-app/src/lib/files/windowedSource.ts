// Tier detection and the open decision for large files (ADR-036 decisions 4-7). Pure: no React, no I/O, no file text.
import type { BufferSource, ReadOnlyKind } from "@/lib/files/bufferReducer";
import type { AdvertisedOp, FileStat, ReadResult } from "@/lib/files/transport/fileTransport";

/** T0 files (at most this many bytes) open whole, exactly as without range reads. */
export const T0_MAX_BYTES = 1024 * 1024;

export type Tier = "t0" | "t1" | "t2";

/** T1 is windowed and (later) editable up to the server's hash cap; T2 is windowed and view-only. */
export function classifyTier(size: number, hashCapBytes: number): Tier {
  if (size <= T0_MAX_BYTES) return "t0";
  return size <= hashCapBytes ? "t1" : "t2";
}

export interface OpenInputs {
  size: number;
  readOnly: ReadOnlyKind;
  analysis: FileStat["analysis"];
  ops: readonly AdvertisedOp[];
  hashCapBytes: number;
  /** The files:range-read flag. */
  rangeRead: boolean;
}

export interface OpenDecision { source: BufferSource; readOnly: ReadOnlyKind }

/**
 * Windowed only when range reads are on and served and the file is over 1 MiB text; everything else keeps the
 * base-plan state. Windowed buffers are view-only (`too_large`) until large-file editing lands (Epic 2.x), which
 * is also the reason a T1 file with files:edit-large or files:delta-write off reads the same.
 */
export function decideOpen(i: OpenInputs): OpenDecision {
  const full: OpenDecision = { source: { kind: "full" }, readOnly: i.readOnly };
  if (!i.rangeRead || !i.ops.includes("read_lines")) return full;
  if (i.readOnly === "binary" || i.readOnly === "non_utf8") return full;
  const tier = classifyTier(i.size, i.hashCapBytes);
  if (tier === "t0") return full;
  return { source: { kind: "windowed", tier, size: i.size, windowStartLine: 1, analysis: i.analysis }, readOnly: "too_large" };
}

export interface LineCount { count: number; exact: boolean }

/** The server's exact count when it has one; at end of file the last line number; otherwise size over the window's bytes per line. */
export function lineCountOf(size: number, r: ReadResult): LineCount {
  if (r.lineCount !== undefined) return { count: r.lineCount, exact: true };
  const { startLine, endLine, startByte, endByte, eof } = r.range;
  if (eof && !r.partialLastLine) return { count: endLine, exact: true };
  const lines = Math.max(endLine - startLine + 1, 1);
  const perLine = Math.max((endByte - startByte) / lines, 1);
  return { count: Math.max(endLine, Math.round(size / perLine)), exact: false };
}

/** "4.2 million" from a million lines up, thousands separators below. */
export function approxLines(n: number): string {
  if (n < 1_000_000) return n.toLocaleString("en-US");
  return `${(n / 1_000_000).toFixed(1).replace(/\.0$/, "")} million`;
}
