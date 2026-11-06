/**
 * Caps live WebGL contexts across terminals. Browsers drop the oldest context past a per-page
 * limit (commonly cited as ~16, lower on some GPUs; not verified here), which can blank those
 * panes; evicting deliberately lets us choose the victim (hidden, then least recently used) and
 * swap it to another renderer first. An evicted pane stays on that renderer for its lifetime.
 */
export const MAX_WEBGL_CONTEXTS = 8;

export interface WebglSlot {
  isVisible(): boolean;
  /** Give up the WebGL context (e.g. swap to the canvas renderer). */
  release(): void;
}

// Insertion order doubles as recency: touch() re-inserts, so the first entry is the oldest.
const slots = new Set<WebglSlot>();

/** Registers `slot`, then evicts others until at most `max` remain. Prefers hidden victims. */
export function claimWebglSlot(slot: WebglSlot, max = MAX_WEBGL_CONTEXTS): void {
  slots.delete(slot);
  slots.add(slot);
  while (slots.size > max) {
    const others = [...slots].filter((s) => s !== slot);
    const victim = others.find((s) => !s.isVisible()) ?? others[0];
    if (!victim) return;
    slots.delete(victim);
    try {
      victim.release();
    } catch (err) {
      // A victim failing to swap must not abort the newcomer's own setup.
      console.error("[webglBudget] failed to release evicted WebGL slot", err);
    }
  }
}

/** Marks `slot` most recently used (call on focus) so it is evicted last. */
export function touchWebglSlot(slot: WebglSlot): void {
  if (!slots.delete(slot)) return;
  slots.add(slot);
}

export function releaseWebglSlot(slot: WebglSlot): void {
  slots.delete(slot);
}
