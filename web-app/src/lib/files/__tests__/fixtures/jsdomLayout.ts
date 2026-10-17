/** jsdom has no layout; CodeMirror's measure pass needs these Range methods to exist. */
export function installJsdomLayoutShims(): void {
  const emptyRects = () => Object.assign([], { item: () => null }) as unknown as DOMRectList;
  const zero = () => new DOMRect(0, 0, 0, 0);
  Range.prototype.getClientRects ??= emptyRects;
  Range.prototype.getBoundingClientRect ??= zero;
}

/** Gives every element a 300x400 box so @tanstack/react-virtual mounts a window of rows; returns the restore function. */
export function installVirtualViewport(): () => void {
  const proto = HTMLElement.prototype;
  const old = { h: Object.getOwnPropertyDescriptor(proto, "offsetHeight"), w: Object.getOwnPropertyDescriptor(proto, "offsetWidth") };
  Object.defineProperty(proto, "offsetHeight", { configurable: true, get: () => 400 });
  Object.defineProperty(proto, "offsetWidth", { configurable: true, get: () => 300 });
  return () => {
    if (old.h) Object.defineProperty(proto, "offsetHeight", old.h); else delete (proto as { offsetHeight?: unknown }).offsetHeight;
    if (old.w) Object.defineProperty(proto, "offsetWidth", old.w); else delete (proto as { offsetWidth?: unknown }).offsetWidth;
  };
}
