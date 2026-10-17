/** jsdom has no layout; CodeMirror's measure pass needs these Range methods to exist. */
export function installJsdomLayoutShims(): void {
  const emptyRects = () => Object.assign([], { item: () => null }) as unknown as DOMRectList;
  const zero = () => new DOMRect(0, 0, 0, 0);
  Range.prototype.getClientRects ??= emptyRects;
  Range.prototype.getBoundingClientRect ??= zero;
}
