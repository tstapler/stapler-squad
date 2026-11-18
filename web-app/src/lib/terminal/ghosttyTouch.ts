/**
 * Touch support for ghostty-web, which only listens to mouse events (and swallows touchend, so the browser
 * never synthesizes a click): tap opens the link menu, long-press then drag selects, long-press alone
 * selects the word, and a drag scrolls (replayed as wheel events, so ghostty's own wheel handling and the
 * mouse-tracking forwarder apply). Selection is driven by replaying mouse events at the canvas so ghostty's own
 * SelectionManager stays the single owner of selection state.
 */
import type { Terminal as GhosttyWebTerminal } from "ghostty-web";

export const LONG_PRESS_MS = 450;
export const TOUCH_SLOP_PX = 10;
const CONTEXT_MENU_SUPPRESS_MS = 800;

interface GhosttyInternals {
  getScrollbackLength(): number;
  getViewportY(): number;
  linkDetector?: { getLinkAt(col: number, row: number): Promise<{ text: string } | undefined> };
  renderer?: { charWidth: number; charHeight: number };
}

/** The URI under a viewport point, using ghostty's own link providers (OSC 8 and URL regex). Same row math as its click handler. */
export async function linkUriAt(term: GhosttyWebTerminal, clientX: number, clientY: number): Promise<string | undefined> {
  const t = term as unknown as GhosttyInternals;
  const canvas = term.element?.querySelector("canvas");
  if (!canvas || !t.renderer || !t.linkDetector) return undefined;
  const rect = canvas.getBoundingClientRect();
  const col = Math.floor((clientX - rect.left) / t.renderer.charWidth);
  const viewRow = Math.floor((clientY - rect.top) / t.renderer.charHeight);
  const scrollback = t.getScrollbackLength();
  const top = Math.max(0, Math.floor(t.getViewportY()));
  const row = top > 0 ? (viewRow < top ? scrollback - top + viewRow : scrollback + (viewRow - top)) : scrollback + viewRow;
  return (await t.linkDetector.getLinkAt(col, row))?.text;
}

export interface GhosttyTouchOptions {
  onLink: (uri: string, x: number, y: number) => void;
}

export function attachGhosttyTouch(term: GhosttyWebTerminal, { onLink }: GhosttyTouchOptions): () => void {
  const el = term.element;
  if (!el) return () => {};

  let start: { x: number; y: number; at: number } | null = null;
  let lastY = 0;
  let moved = false;
  let selecting = false;
  let timer: ReturnType<typeof setTimeout> | undefined;
  let suppressContextMenuUntil = 0;

  const canvas = () => el.querySelector("canvas");
  const mouse = (type: string, x: number, y: number) =>
    new MouseEvent(type, { bubbles: true, cancelable: true, clientX: x, clientY: y, button: 0, buttons: type === "mouseup" ? 0 : 1 });
  const replay = (type: string, x: number, y: number, target: EventTarget | null = canvas()) => target?.dispatchEvent(mouse(type, x, y));
  const reset = () => {
    clearTimeout(timer);
    start = null;
    moved = false;
    selecting = false;
  };

  const onTouchStart = (e: TouchEvent) => {
    reset();
    if (e.touches.length !== 1 || e.target !== canvas()) return;
    const { clientX: x, clientY: y } = e.touches[0];
    start = { x, y, at: Date.now() };
    lastY = y;
    timer = setTimeout(() => {
      selecting = true;
      suppressContextMenuUntil = Date.now() + CONTEXT_MENU_SUPPRESS_MS;
      navigator.vibrate?.(10);
      replay("mousedown", x, y);
    }, LONG_PRESS_MS);
  };

  const onTouchMove = (e: TouchEvent) => {
    if (!start) return;
    const { clientX: x, clientY: y } = e.touches[0];
    if (Math.hypot(x - start.x, y - start.y) > TOUCH_SLOP_PX) {
      moved = true;
      if (!selecting) clearTimeout(timer);
    }
    if (selecting) {
      e.preventDefault();
      replay("mousemove", x, y);
      return;
    }
    if (!moved) return;
    e.preventDefault();
    // Finger down moves content down, which is a negative wheel delta.
    canvas()?.dispatchEvent(new WheelEvent("wheel", { bubbles: true, cancelable: true, deltaY: lastY - y, deltaMode: 0, clientX: x, clientY: y }));
    lastY = y;
  };

  const finish = (e: TouchEvent, cancelled: boolean) => {
    const s = start;
    const wasSelecting = selecting;
    const wasMoved = moved;
    const touch = e.changedTouches[0];
    reset();
    if (!s || !touch) return;
    if (wasSelecting) {
      replay("mouseup", touch.clientX, touch.clientY, document);
      if (!cancelled && !wasMoved) replay("dblclick", s.x, s.y);
      return;
    }
    if (cancelled || wasMoved) return;
    if (term.hasSelection()) term.clearSelection();
    void linkUriAt(term, s.x, s.y).then((uri) => uri && onLink(uri, s.x, s.y));
  };
  const onTouchEnd = (e: TouchEvent) => finish(e, false);
  const onTouchCancel = (e: TouchEvent) => finish(e, true);

  // Android fires contextmenu after a long-press; the long-press already selected, so the menu would only cover it.
  const onContextMenu = (e: MouseEvent) => {
    if (!selecting && Date.now() > suppressContextMenuUntil) return;
    e.preventDefault();
    e.stopImmediatePropagation();
  };

  el.addEventListener("touchstart", onTouchStart, { passive: true });
  el.addEventListener("touchmove", onTouchMove, { passive: false });
  el.addEventListener("touchend", onTouchEnd);
  el.addEventListener("touchcancel", onTouchCancel);
  el.addEventListener("contextmenu", onContextMenu, true);
  return () => {
    reset();
    el.removeEventListener("touchstart", onTouchStart);
    el.removeEventListener("touchmove", onTouchMove);
    el.removeEventListener("touchend", onTouchEnd);
    el.removeEventListener("touchcancel", onTouchCancel);
    el.removeEventListener("contextmenu", onContextMenu, true);
  };
}
