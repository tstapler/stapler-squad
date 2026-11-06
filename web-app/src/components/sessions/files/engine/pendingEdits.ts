import type { ChangeSet, Text } from "@codemirror/state";
import type { Edit, EditPos } from "./EditorEngine";

function posAt(base: Text, offset: number): EditPos {
  const line = base.lineAt(offset);
  return { line: line.number - 1, colUtf16: offset - line.from };
}

/** `changes` addresses `base`; the result is in `base` coordinates, ordered and non-overlapping. */
export function changesToEdits(changes: ChangeSet, base: Text): Edit[] {
  const edits: Edit[] = [];
  changes.iterChanges((fromA, toA, _fromB, _toB, inserted) => {
    edits.push({ start: posAt(base, fromA), end: posAt(base, toA), insert: inserted.sliceString(0, undefined, "\n") });
  });
  return edits;
}
