/**
 * The y coordinate (px) of the bottom edge of the session tab row, or null when
 * no session page is mounted. The session layout publishes it; the mobile toast
 * deck docks directly below it (ADR-009) and falls back to the bottom dock
 * when it is null. Mirrored to `--mobile-stack-top-offset` for CSS.
 */
let topOffsetPx: number | null = null;
const listeners = new Set<() => void>();

export function setStackTopOffset(px: number | null): void {
  if (px === topOffsetPx) return;
  topOffsetPx = px;
  if (typeof document !== "undefined") {
    document.documentElement.style.setProperty("--mobile-stack-top-offset", `${px ?? 0}px`);
  }
  listeners.forEach((l) => l());
}

export function getStackTopOffset(): number | null {
  return topOffsetPx;
}

export function subscribeStackTopOffset(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}
