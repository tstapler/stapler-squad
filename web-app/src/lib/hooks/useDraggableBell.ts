"use client";

import { useCallback, useEffect, useRef, useState, type CSSProperties, type KeyboardEvent, type PointerEvent } from "react";
import { useAnnounce } from "@/lib/hooks/useAnnounce";
import { useHandedness } from "@/lib/hooks/useHandedness";
import {
  BELL_SETTINGS_EVENT,
  readBellPos,
  readFloatingBell,
  resetBellPos,
  writeBellPos,
} from "@/lib/utils/deckSettings";
import {
  DESKTOP_DEFAULT_Y_FRACTION_OF_VIEWPORT,
  PHONE_DEFAULT_Y,
  clampTop,
  computeBounds,
  describeY,
  movedPastThreshold,
  stepY,
  topToY,
  yToTop,
  type BellBounds,
  type BellEdge,
  type BellPos,
} from "@/lib/utils/bellPosition";

const FALLBACK_HANDLE_HEIGHT = 88;
/** Special-keys bar sits above the bottom nav on a phone terminal. */
const PHONE_KEYS_BAR_RESERVE = 48;

function cssPx(name: string): number {
  if (typeof document === "undefined") return 0;
  const n = parseFloat(getComputedStyle(document.documentElement).getPropertyValue(name));
  return Number.isFinite(n) ? n : 0;
}

/** env() cannot be read from JS directly; a probe element resolves it. */
function safeInset(side: "top" | "bottom"): number {
  if (typeof document === "undefined") return 0;
  const probe = document.createElement("div");
  probe.style.cssText = `position:fixed;visibility:hidden;padding-${side}:env(safe-area-inset-${side}, 0px)`;
  document.body.appendChild(probe);
  const n = parseFloat(getComputedStyle(probe).getPropertyValue(`padding-${side}`));
  probe.remove();
  return Number.isFinite(n) ? n : 0;
}

interface SafeInsets {
  top: number;
  bottom: number;
}

function measureSafeInsets(): SafeInsets {
  return { top: safeInset("top"), bottom: safeInset("bottom") };
}

function measureBounds(phone: boolean, handleHeight: number, insets: SafeInsets): BellBounds {
  // Reached during render, so it must survive the server prerender (no window).
  const viewportHeight =
    cssPx("--viewport-height") || (typeof window === "undefined" ? 0 : window.visualViewport?.height || window.innerHeight);
  return computeBounds({
    viewportHeight,
    safeTop: insets.top,
    safeBottom: insets.bottom,
    bottomInset: phone ? cssPx("--bottom-nav-height") + PHONE_KEYS_BAR_RESERVE : 0,
    handleHeight,
  });
}

interface DragState {
  pointerId: number;
  startX: number;
  startY: number;
  startTop: number;
  pointerType: string;
  active: boolean;
}

export interface DraggableBell {
  /** Inline overrides for the handle's own CSS; empty until the user (or the phone default) moves it. */
  style: CSSProperties;
  edge: BellEdge;
  dragging: boolean;
  handlers: {
    onPointerDown: (e: PointerEvent<HTMLElement>) => void;
    onPointerMove: (e: PointerEvent<HTMLElement>) => void;
    onPointerUp: (e: PointerEvent<HTMLElement>) => void;
    onPointerCancel: (e: PointerEvent<HTMLElement>) => void;
    onKeyDown: (e: KeyboardEvent<HTMLElement>) => void;
  };
  /** True once after a drag so the trailing click does not toggle the tray. */
  consumeClickSuppression: () => boolean;
  /** Whether the phone floating bell is switched on for this device. */
  floatingBellEnabled: boolean;
}

/**
 * Click-and-drag repositioning for the tray handle. Plain pointer events: a press
 * only becomes a drag past a small threshold, so a tap still toggles the tray.
 * Desktop is right-edge, vertical only; a phone (opt-in) snaps to the thumb side.
 */
export function useDraggableBell(ref: React.RefObject<HTMLElement | null>, phone: boolean): DraggableBell {
  const { announce } = useAnnounce();
  const { leftHanded } = useHandedness();
  const [stored, setStored] = useState<BellPos | null>(() => (typeof window === "undefined" ? null : readBellPos()));
  const [floatingBellEnabled, setFloatingBellEnabled] = useState(() => typeof window !== "undefined" && readFloatingBell());
  const [dragTop, setDragTop] = useState<number | null>(null);
  const [, setLayoutTick] = useState(0);
  const drag = useRef<DragState | null>(null);
  const suppressClick = useRef(false);
  // Probed in effects only: render must not touch the DOM.
  const insets = useRef<SafeInsets>({ top: 0, bottom: 0 });

  useEffect(() => {
    const reread = () => {
      setStored(readBellPos());
      setFloatingBellEnabled(readFloatingBell());
    };
    const relayout = () => {
      insets.current = measureSafeInsets();
      setLayoutTick((n) => n + 1);
    };
    relayout();
    window.addEventListener(BELL_SETTINGS_EVENT, reread);
    window.addEventListener("resize", relayout);
    window.addEventListener("orientationchange", relayout);
    window.visualViewport?.addEventListener("resize", relayout);
    return () => {
      window.removeEventListener(BELL_SETTINGS_EVENT, reread);
      window.removeEventListener("resize", relayout);
      window.removeEventListener("orientationchange", relayout);
      window.visualViewport?.removeEventListener("resize", relayout);
    };
  }, []);

  const handleHeight = () => ref.current?.offsetHeight || FALLBACK_HANDLE_HEIGHT;
  const edge: BellEdge = phone ? stored?.edge ?? (leftHanded ? "left" : "right") : "right";

  const currentY = useCallback(
    (b: BellBounds): number => {
      if (stored) return stored.y;
      if (phone) return PHONE_DEFAULT_Y;
      const top = ref.current?.getBoundingClientRect().top ?? window.innerHeight * DESKTOP_DEFAULT_Y_FRACTION_OF_VIEWPORT;
      return topToY(top, b);
    },
    [stored, phone, ref],
  );

  const commit = (y: number, announceMove: boolean) => {
    const pos: BellPos = { v: 1, edge, y };
    setStored(pos);
    writeBellPos(pos);
    if (announceMove) announce(`Notification button moved to ${describeY(y)}`, "polite", "bell-moved");
  };

  const onPointerDown = (e: PointerEvent<HTMLElement>) => {
    if (e.button !== undefined && e.button !== 0) return;
    const rect = e.currentTarget.getBoundingClientRect();
    drag.current = {
      pointerId: e.pointerId,
      startX: e.clientX,
      startY: e.clientY,
      startTop: rect.top,
      pointerType: e.pointerType || "mouse",
      active: false,
    };
    try {
      e.currentTarget.setPointerCapture?.(e.pointerId);
    } catch {
      // Capture is best effort; move/up still arrive while the pointer stays over the handle.
    }
  };

  const onPointerMove = (e: PointerEvent<HTMLElement>) => {
    const d = drag.current;
    if (!d || d.pointerId !== e.pointerId) return;
    const dx = e.clientX - d.startX;
    const dy = e.clientY - d.startY;
    if (!d.active) {
      if (!movedPastThreshold(dx, dy, d.pointerType)) return;
      d.active = true;
    }
    setDragTop(clampTop(d.startTop + dy, measureBounds(phone, handleHeight(), insets.current)));
  };

  const finish = (e: PointerEvent<HTMLElement>) => {
    const d = drag.current;
    if (!d || d.pointerId !== e.pointerId) return;
    drag.current = null;
    try {
      e.currentTarget.releasePointerCapture?.(e.pointerId);
    } catch {
      // Already released.
    }
    if (!d.active) return;
    const b = measureBounds(phone, handleHeight(), insets.current);
    const top = clampTop(d.startTop + (e.clientY - d.startY), b);
    suppressClick.current = true;
    setTimeout(() => (suppressClick.current = false), 0);
    setDragTop(null);
    commit(topToY(top, b), false);
  };

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
    e.preventDefault();
    const b = measureBounds(phone, handleHeight(), insets.current);
    commit(stepY(currentY(b), e.key === "ArrowUp" ? "up" : "down", e.shiftKey), true);
  };

  const consumeClickSuppression = () => {
    const was = suppressClick.current;
    suppressClick.current = false;
    return was;
  };

  const style: CSSProperties = {};
  if (dragTop !== null || stored || phone) {
    const b = measureBounds(phone, handleHeight(), insets.current);
    style.top = dragTop ?? yToTop(stored ? stored.y : PHONE_DEFAULT_Y, b);
    if (dragTop !== null) style.transition = "none";
  }
  if (phone && edge === "left") {
    Object.assign(style, {
      left: 0,
      right: "auto",
      borderLeft: "none",
      borderRight: "1px solid var(--border-color, currentColor)",
      borderRadius: "0 8px 8px 0",
    });
  }

  return {
    style,
    edge,
    dragging: dragTop !== null,
    handlers: { onPointerDown, onPointerMove, onPointerUp: finish, onPointerCancel: finish, onKeyDown },
    consumeClickSuppression,
    floatingBellEnabled,
  };
}

export { resetBellPos };
