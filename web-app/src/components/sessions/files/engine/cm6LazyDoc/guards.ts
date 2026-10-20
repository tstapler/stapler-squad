import { EditorState, type Extension, type Transaction } from "@codemirror/state";
import { gapTableOf, isLazyLoad, type GapTable } from "./gapTable";

/** True when any change overlaps a placeholder line or the newline joining it to a neighbor. */
function touchesPlaceholder(table: GapTable, tr: Transaction): boolean {
  let bad = false;
  tr.changes.iterChangedRanges((fromA, toA) => {
    for (const s of table.segs) {
      if (s.kind !== "gap") continue;
      const start = Math.max(0, s.from - 1);
      const end = s.to + 1;
      const hit = toA > fromA ? fromA < end && toA > start : fromA > start && fromA < end;
      if (hit) bad = true;
    }
  });
  return bad;
}

/** Refuses user edits into placeholders; load and evict transactions are the only writers there. */
export const placeholderGuard: Extension = EditorState.changeFilter.of(
  (tr) => isLazyLoad(tr) || !touchesPlaceholder(gapTableOf(tr.startState), tr),
);
