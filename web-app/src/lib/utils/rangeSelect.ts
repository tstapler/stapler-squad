type SessionFlatItem = { kind: "session"; session: { id: string } };
type FlatItem = { kind: string; session?: { id: string } };

export function computeRangeIds(
  anchorId: string,
  targetId: string,
  flatItems: FlatItem[]
): string[] {
  const anchorIdx = flatItems.findIndex(i => i.kind === "session" && i.session?.id === anchorId);
  const targetIdx = flatItems.findIndex(i => i.kind === "session" && i.session?.id === targetId);
  if (anchorIdx === -1 || targetIdx === -1) return [targetId];
  const lo = Math.min(anchorIdx, targetIdx);
  const hi = Math.max(anchorIdx, targetIdx);
  return flatItems
    .slice(lo, hi + 1)
    .filter(i => i.kind === "session")
    .map(i => (i as SessionFlatItem).session.id);
}
