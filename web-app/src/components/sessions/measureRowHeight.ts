/**
 * Virtualizer `measureElement` for variable-height rows. Prefers the
 * ResizeObserver entry's border-box size (no layout read); the first measure of
 * a mounted row has no entry, so fall back to `offsetHeight` rather than
 * `getBoundingClientRect()` (an allocation plus a forced layout per row).
 */
export function measureRowHeight(
  el: Element,
  entry?: Pick<ResizeObserverEntry, "borderBoxSize"> | null,
): number {
  const box = entry?.borderBoxSize?.[0];
  if (box) return Math.round(box.blockSize);
  return (el as HTMLElement).offsetHeight;
}
