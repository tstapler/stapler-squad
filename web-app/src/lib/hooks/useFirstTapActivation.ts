"use client";

import { useCallback, useRef, type MouseEvent, type PointerEvent } from "react";

/** Beyond this the gesture is a scroll, not a tap (ux.md 3.5). */
export const TAP_SLOP_PX = 10;
const CLICK_SWALLOW_MS = 600;

interface Down { key: string; x: number; y: number; pointerId: number }

const rowKeyOf = (target: EventTarget | null): string | undefined =>
  (target as HTMLElement | null)?.closest?.<HTMLElement>("[data-row-key]")?.dataset.rowKey;

/**
 * One tap opens a row even while the soft keyboard is up: the row is captured on pointerdown (the input is
 * blurred then, so the keyboard closes), and activated on pointerup without hit-testing again, because the
 * viewport relayout between the two can move a different row under the finger. A pointer that travels more
 * than 10 px is a scroll. A click with no pointer sequence (assistive technology) still activates.
 * Spread the result on the element containing rows that carry `data-row-key`.
 */
export function useFirstTapActivation(onActivate: (key: string) => void) {
  const down = useRef<Down | null>(null);
  const swallowClick = useRef(false);
  const swallowTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const onPointerDown = useCallback((e: PointerEvent<HTMLElement>) => {
    const key = rowKeyOf(e.target);
    if (!key) return;
    down.current = { key, x: e.clientX, y: e.clientY, pointerId: e.pointerId };
    const focused = document.activeElement;
    if (focused instanceof HTMLInputElement || focused instanceof HTMLTextAreaElement) focused.blur();
  }, []);

  const onPointerMove = useCallback((e: PointerEvent<HTMLElement>) => {
    const d = down.current;
    if (d && d.pointerId === e.pointerId && Math.hypot(e.clientX - d.x, e.clientY - d.y) > TAP_SLOP_PX) down.current = null;
  }, []);

  const onPointerUp = useCallback((e: PointerEvent<HTMLElement>) => {
    const d = down.current;
    down.current = null;
    if (!d || d.pointerId !== e.pointerId || Math.hypot(e.clientX - d.x, e.clientY - d.y) > TAP_SLOP_PX) return;
    swallowClick.current = true;
    if (swallowTimer.current !== null) clearTimeout(swallowTimer.current);
    swallowTimer.current = setTimeout(() => { swallowClick.current = false; }, CLICK_SWALLOW_MS);
    onActivate(d.key);
  }, [onActivate]);

  const onPointerCancel = useCallback(() => { down.current = null; }, []);

  const onClick = useCallback((e: MouseEvent<HTMLElement>) => {
    if (swallowClick.current) {
      swallowClick.current = false;
      return;
    }
    const key = rowKeyOf(e.target);
    if (key) onActivate(key);
  }, [onActivate]);

  return { onPointerDown, onPointerMove, onPointerUp, onPointerCancel, onClick };
}
