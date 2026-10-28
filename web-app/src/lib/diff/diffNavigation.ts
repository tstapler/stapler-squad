import type { DiffFile, DiffLine } from "@/lib/utils/parseDiff";

export interface HunkRef {
  fileIndex: number;
  hunkIndex: number;
}

/** Stable DOM id for a hunk, shared by DiffRenderer (renders it) and callers (scroll targets). */
export function hunkDomId(fileIndex: number, hunkIndex: number): string {
  return `diff-hunk-${fileIndex}-${hunkIndex}`;
}

/** Flat, document-ordered list of every hunk across all files. */
export function buildHunkList(files: DiffFile[]): HunkRef[] {
  const refs: HunkRef[] = [];
  files.forEach((f, fileIndex) => f.changes.forEach((_, hunkIndex) => refs.push({ fileIndex, hunkIndex })));
  return refs;
}

/** Next/prev index with wraparound; `current` null means "nothing active yet". Returns null when there are no hunks. */
export function stepHunkIndex(current: number | null, count: number, direction: "next" | "prev"): number | null {
  if (count === 0) return null;
  if (current === null || current < 0 || current >= count) return direction === "next" ? 0 : count - 1;
  return direction === "next" ? (current + 1) % count : (current - 1 + count) % count;
}

export interface SplitRow {
  left: DiffLine | null;
  right: DiffLine | null;
}

/**
 * Align a hunk's lines into side-by-side rows. Context lines span both sides;
 * a run of deletes followed by adds is paired index-by-index, with the shorter
 * side padded by null so both columns stay row-aligned.
 */
export function pairSplitRows(lines: DiffLine[]): SplitRow[] {
  const rows: SplitRow[] = [];
  let i = 0;
  while (i < lines.length) {
    if (lines[i].type === "context") {
      rows.push({ left: lines[i], right: lines[i] });
      i++;
      continue;
    }
    const deletes: DiffLine[] = [];
    const adds: DiffLine[] = [];
    while (i < lines.length && lines[i].type === "delete") deletes.push(lines[i++]);
    while (i < lines.length && lines[i].type === "add") adds.push(lines[i++]);
    const n = Math.max(deletes.length, adds.length);
    for (let k = 0; k < n; k++) rows.push({ left: deletes[k] ?? null, right: adds[k] ?? null });
  }
  return rows;
}
