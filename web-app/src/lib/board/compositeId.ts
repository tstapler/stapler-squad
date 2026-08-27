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
 * Splits a composite `${rowKey}:${entityId}` id on the first `:` — session IDs and
 * BoardColumnKey values never contain `:`, so this is unambiguous even if a future rowKey
 * (e.g. a branch name) did.
 */
export function parseCompositeId(id: string): { rowKey: string; entityId: string } {
  const idx = id.indexOf(":");
  if (idx === -1) {
    return { rowKey: "", entityId: id };
  }
  return { rowKey: id.slice(0, idx), entityId: id.slice(idx + 1) };
}
