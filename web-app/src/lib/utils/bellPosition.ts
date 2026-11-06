/** Pure geometry and parsing for the draggable notification bell (no DOM, no storage). */
export type BellEdge = "left" | "right";
export interface BellPos {
  v: 1;
  edge: BellEdge;
  /** 0 = top of the usable range, 1 = bottom. */
  y: number;
}

export interface BellBounds {
  minTop: number;
  maxTop: number;
}

export const BELL_EDGE_MARGIN_PX = 8;
export const DESKTOP_DEFAULT_Y_FRACTION_OF_VIEWPORT = 0.35;
export const PHONE_DEFAULT_Y = 0.8;
export const KEY_STEP = 0.05;
export const KEY_STEP_LARGE = 0.2;
const MOUSE_THRESHOLD_PX = 6;
const TOUCH_THRESHOLD_PX = 10;

const clamp01 = (n: number) => Math.min(1, Math.max(0, n));

/** Returns null for anything that is not a well-formed v1 record. */
export function parseBellPos(raw: string | null): BellPos | null {
  if (!raw) return null;
  try {
    const o = JSON.parse(raw) as Record<string, unknown>;
    if (o?.v !== 1 || (o.edge !== "left" && o.edge !== "right")) return null;
    if (typeof o.y !== "number" || !Number.isFinite(o.y)) return null;
    return { v: 1, edge: o.edge, y: clamp01(o.y) };
  } catch {
    return null;
  }
}

export function computeBounds(p: {
  viewportHeight: number;
  safeTop: number;
  safeBottom: number;
  /** Bottom nav (and keys bar) height on a phone, else 0. */
  bottomInset: number;
  handleHeight: number;
}): BellBounds {
  const minTop = p.safeTop + BELL_EDGE_MARGIN_PX;
  const maxTop = p.viewportHeight - p.safeBottom - p.bottomInset - BELL_EDGE_MARGIN_PX - p.handleHeight;
  return { minTop, maxTop: Math.max(minTop, maxTop) };
}

export function yToTop(y: number, b: BellBounds): number {
  return b.minTop + clamp01(y) * (b.maxTop - b.minTop);
}

export function topToY(top: number, b: BellBounds): number {
  const range = b.maxTop - b.minTop;
  return range <= 0 ? 0 : clamp01((top - b.minTop) / range);
}

export function clampTop(top: number, b: BellBounds): number {
  return Math.min(b.maxTop, Math.max(b.minTop, top));
}

export function dragThreshold(pointerType: string): number {
  return pointerType === "mouse" ? MOUSE_THRESHOLD_PX : TOUCH_THRESHOLD_PX;
}

export function movedPastThreshold(dx: number, dy: number, pointerType: string): boolean {
  return Math.hypot(dx, dy) >= dragThreshold(pointerType);
}

export function stepY(y: number, direction: "up" | "down", large: boolean): number {
  const step = large ? KEY_STEP_LARGE : KEY_STEP;
  return clamp01(y + (direction === "down" ? step : -step));
}

export function describeY(y: number): "top" | "middle" | "bottom" {
  return y < 1 / 3 ? "top" : y < 2 / 3 ? "middle" : "bottom";
}
