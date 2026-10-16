/**
 * dnd-kit requires unique ids for every draggable/droppable within one DndContext. Once
 * multiple swimlane rows each render the same 4 column keys (and Tag grouping renders one
 * session's card in multiple rows simultaneously), a raw `column.key`/`session.id` id scheme
 * collides. Every BoardColumn/BoardCard id is scoped by rowKey as `${rowKey}:${entityId}` —
 * `"__default__"` until real swimlane keys land — so the scheme is uniform from Phase 3
 * onward, not retrofitted later.
 */
export function makeCompositeId(rowKey: string, entityId: string): string {
  return `${rowKey}:${entityId}`;
}

/**
 * Splits a composite `${rowKey}:${entityId}` id on the LAST `:` — session IDs and
 * BoardColumnKey values never contain `:`, but rowKey does once Phase 6 wires real
 * grouping-strategy values through (a tag/category/path string can legitimately contain a
 * colon, e.g. a Windows-style path or a "type:bug" tag). Splitting on the last occurrence
 * keeps parsing correct regardless of what the rowKey contains.
 */
export function parseCompositeId(id: string): { rowKey: string; entityId: string } {
  const idx = id.lastIndexOf(":");
  if (idx === -1) {
    return { rowKey: "", entityId: id };
  }
  return { rowKey: id.slice(0, idx), entityId: id.slice(idx + 1) };
}
