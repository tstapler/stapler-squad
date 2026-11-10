import type { Terminal } from "@xterm/xterm";

type ScrollbarTerm = Pick<Terminal, "buffer" | "rows">;

export interface ScrollbarUpdaterDeps {
  getTrack: () => HTMLElement | null;
  getThumb: () => HTMLElement | null;
  requestFrame?: (cb: () => void) => number;
  cancelFrame?: (id: number) => void;
}

export interface ScrollbarUpdater {
  /** Remember the newest terminal state and apply it once on the next frame. */
  schedule(term: ScrollbarTerm): void;
  /** Apply now (initial fit), cancelling any pending frame. */
  flush(term?: ScrollbarTerm): void;
  /** Forget the cached track height; call when the terminal or its container may have resized. */
  invalidateTrackHeight(): void;
  dispose(): void;
}

/**
 * Keeps the custom scrollbar thumb in sync with the viewport. onWriteParsed fires for every
 * output chunk, and the old per-chunk update wrote display/height/transform and then read
 * clientHeight, forcing a synchronous layout each time (the top JS self-time function under
 * streaming output). This coalesces to one apply per frame, skips unchanged style writes, and
 * reads the track height from layout only after an invalidation (resize), not per chunk.
 */
export function createScrollbarUpdater(deps: ScrollbarUpdaterDeps): ScrollbarUpdater {
  const requestFrame = deps.requestFrame ?? ((cb) => window.requestAnimationFrame(cb));
  const cancelFrame = deps.cancelFrame ?? ((id) => window.cancelAnimationFrame(id));
  let pending: ScrollbarTerm | null = null;
  let frameId: number | null = null;
  let trackHeight = 0;
  let lastDisplay = "";
  let lastHeight = "";
  let lastTransform = "";

  const apply = (term: ScrollbarTerm) => {
    const track = deps.getTrack();
    const thumb = deps.getThumb();
    if (!track || !thumb) return;
    const buf = term.buffer.active;
    const totalLines = buf.length;
    const visibleLines = term.rows;
    if (totalLines <= visibleLines) {
      if (lastDisplay !== "none") track.style.display = lastDisplay = "none";
      trackHeight = 0; // a hidden track measures 0, so re-read once it shows again
      return;
    }
    if (lastDisplay !== "block") track.style.display = lastDisplay = "block";
    if (trackHeight <= 0) trackHeight = track.clientHeight;
    const height = trackHeight;
    if (height === 0) return;
    const thumbHeight = Math.max(20, height * (visibleLines / totalLines));
    const maxScrollLines = totalLines - visibleLines;
    const progress = maxScrollLines > 0 ? buf.viewportY / maxScrollLines : 0;
    const heightCss = `${thumbHeight}px`;
    const transformCss = `translateY(${progress * (height - thumbHeight)}px)`;
    if (heightCss !== lastHeight) thumb.style.height = lastHeight = heightCss;
    if (transformCss !== lastTransform) thumb.style.transform = lastTransform = transformCss;
  };

  const cancel = () => {
    if (frameId !== null) cancelFrame(frameId);
    frameId = null;
  };

  return {
    schedule(term) {
      pending = term;
      if (frameId !== null) return;
      frameId = requestFrame(() => {
        frameId = null;
        const t = pending;
        pending = null;
        if (t) apply(t);
      });
    },
    flush(term) {
      const t = term ?? pending;
      pending = null;
      cancel();
      if (t) apply(t);
    },
    invalidateTrackHeight() {
      trackHeight = 0;
    },
    dispose() {
      pending = null;
      cancel();
    },
  };
}
