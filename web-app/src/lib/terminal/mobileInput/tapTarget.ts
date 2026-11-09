/**
 * Layer-owned tap-to-focus. Binds the VISIBLE terminal surface (never the 1px, pointer-events:none
 * textarea) and focuses synchronously inside the user gesture. `pointerup`, `touchend` and `click` are
 * all bound because gesture code may `preventDefault` touches and a scroll ends in `pointercancel`;
 * one tap fires several of them, so triggers inside the dedupe window count as the same tap.
 */
import { TAP_DEDUPE_MS, TAP_SLOP_PX } from "./policy";
import type { MobileInputClock } from "./types";

interface Point {
  x: number;
  y: number;
}

const pointerPoint = (e: Event): Point => ({ x: (e as MouseEvent).clientX ?? 0, y: (e as MouseEvent).clientY ?? 0 });

function touchPoint(e: Event): Point {
  const t = (e as TouchEvent).changedTouches?.[0] ?? (e as TouchEvent).touches?.[0];
  return t ? { x: t.clientX, y: t.clientY } : { x: 0, y: 0 };
}

export function bindTapTarget(surface: HTMLElement, focus: () => void, clock: MobileInputClock): () => void {
  let start: Point | null = null;
  let lastFocusAt: number | null = null;

  const moved = (end: Point) => start !== null && Math.hypot(end.x - start.x, end.y - start.y) > TAP_SLOP_PX;

  const tap = () => {
    const now = clock.now();
    if (lastFocusAt !== null && now - lastFocusAt < TAP_DEDUPE_MS) return;
    lastFocusAt = now;
    focus();
  };

  const handlers: Array<[string, (e: Event) => void]> = [
    ["pointerdown", (e) => ((start = pointerPoint(e)), undefined)],
    ["touchstart", (e) => ((start = touchPoint(e)), undefined)],
    ["pointercancel", () => ((start = null), undefined)],
    ["touchcancel", () => ((start = null), undefined)],
    [
      "pointerup",
      (e) => {
        const type = (e as PointerEvent).pointerType;
        const end = pointerPoint(e);
        const isTap = (type === "touch" || type === "pen") && !moved(end);
        start = null;
        if (isTap) tap();
      },
    ],
    [
      "touchend",
      (e) => {
        const isTap = !moved(touchPoint(e));
        start = null;
        if (isTap) tap();
      },
    ],
    [
      "click",
      (e) => {
        if ((e as PointerEvent).pointerType === "mouse") return;
        tap();
      },
    ],
  ];

  // Capture phase so a gesture hook that stops propagation cannot hide the tap from the layer.
  for (const [type, fn] of handlers) surface.addEventListener(type, fn, { capture: true, passive: true });
  return () => {
    for (const [type, fn] of handlers) surface.removeEventListener(type, fn, { capture: true });
  };
}
