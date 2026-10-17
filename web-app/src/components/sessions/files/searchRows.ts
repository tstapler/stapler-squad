import type { SearchHit } from "@/lib/files/searchClient";

export type ResultRow =
  | { kind: "group"; key: string; path: string; count: number; posinset: number; setsize: number; collapsed: boolean }
  | {
      kind: "hit"; key: string; hit: SearchHit; hitIndex: number;
      /** Position inside the file group; `setsize` is that group's size. */
      posinset: number; setsize: number;
      /** 1-based position over all results, and their total. */
      result: number; total: number;
    };

export const groupKey = (path: string) => `g:${path}`;
export const hitKey = (hitIndex: number) => `h:${hitIndex}`;

export const DIRTY_NOTE = "buffer has unsaved changes; line may differ";

/** Files in first-seen order, each followed by its hits unless collapsed; hit numbering ignores collapsing. */
export function buildRows(hits: readonly SearchHit[], collapsed: ReadonlySet<string>): ResultRow[] {
  const groups = new Map<string, number[]>();
  hits.forEach((h, i) => {
    const list = groups.get(h.path);
    if (list) list.push(i);
    else groups.set(h.path, [i]);
  });
  const rows: ResultRow[] = [];
  let g = 0;
  for (const [path, indexes] of groups) {
    g += 1;
    const isCollapsed = collapsed.has(path);
    rows.push({ kind: "group", key: groupKey(path), path, count: indexes.length, posinset: g, setsize: groups.size, collapsed: isCollapsed });
    if (isCollapsed) continue;
    indexes.forEach((hitIndex, k) => {
      rows.push({ kind: "hit", key: hitKey(hitIndex), hit: hits[hitIndex], hitIndex, posinset: k + 1, setsize: indexes.length, result: hitIndex + 1, total: hits.length });
    });
  }
  return rows;
}

/** The name a screen reader gets; the `<mark>` stays inside the visible text (ux.md 3.5). */
export function accessibleName(row: ResultRow, dirty = false, collapsed = false): string {
  if (row.kind === "group") {
    const noun = row.count === 1 ? "match" : "matches";
    return `${row.path}, ${row.count} ${noun}, ${collapsed || row.collapsed ? "collapsed" : "expanded"}`;
  }
  const base = `${row.hit.path} line ${row.hit.line}, ${row.hit.text}, match ${row.posinset} of ${row.setsize}, result ${row.result} of ${row.total}`;
  return dirty ? `${base}, ${DIRTY_NOTE}` : base;
}
