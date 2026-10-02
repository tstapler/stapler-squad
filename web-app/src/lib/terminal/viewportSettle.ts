/**
 * ViewportSettled signal (Story 2.1.4): fires once both visualViewport height
 * and offsetTop are unchanged for `stableFrames` consecutive frames, or after
 * `maxWaitMs` from the arming event, whichever comes first. Armed by vv
 * `resize` and `scroll`. Pure: scheduler and window are injected; imports
 * nothing from lib/hooks.
 */

export interface ViewportSnapshot {
  height: number;
  offsetTop: number;
}

export interface ViewportSource extends ViewportSnapshot {
  addEventListener(type: "resize" | "scroll", listener: () => void): void;
  removeEventListener(type: "resize" | "scroll", listener: () => void): void;
}

export interface FrameScheduler {
  request(callback: () => void): number;
  cancel(handle: number): void;
  now(): number;
}

export interface WindowLike {
  innerHeight: number;
  addEventListener(type: "resize", listener: () => void): void;
  removeEventListener(type: "resize", listener: () => void): void;
}

export interface ViewportSettleOptions {
  stableFrames: number;
  maxWaitMs: number;
  onSettled: (snapshot: ViewportSnapshot) => void;
  /** Used only when no visualViewport exists; defaults to the global window. */
  win?: WindowLike;
}

export const DEFAULT_STABLE_FRAMES = 3;
export const DEFAULT_MAX_WAIT_MS = 600;

export function createRafScheduler(): FrameScheduler {
  return {
    request: (cb) => requestAnimationFrame(() => cb()),
    cancel: (h) => cancelAnimationFrame(h),
    now: () => performance.now(),
  };
}

function windowFallbackSource(win: WindowLike): ViewportSource {
  return {
    get height() {
      return win.innerHeight;
    },
    offsetTop: 0,
    addEventListener: (type, listener) => {
      if (type === "resize") win.addEventListener("resize", listener);
    },
    removeEventListener: (type, listener) => {
      if (type === "resize") win.removeEventListener("resize", listener);
    },
  };
}

/** Returns a dispose function that unsubscribes and cancels any pending frame. */
export function createViewportSettle(
  vv: ViewportSource | null | undefined,
  scheduler: FrameScheduler,
  options: ViewportSettleOptions
): () => void {
  const source = vv ?? windowFallbackSource(options.win ?? (window as unknown as WindowLike));
  const { stableFrames, maxWaitMs, onSettled } = options;

  let frameHandle: number | null = null;
  let armedAt = 0;
  let lastHeight = NaN;
  let lastOffsetTop = NaN;
  let stableCount = 0;
  let disposed = false;

  const settle = (height: number, offsetTop: number) => {
    frameHandle = null;
    onSettled({ height, offsetTop });
  };

  const onFrame = () => {
    frameHandle = null;
    if (disposed) return;
    const { height, offsetTop } = source;
    if (height === lastHeight && offsetTop === lastOffsetTop) {
      stableCount++;
    } else {
      lastHeight = height;
      lastOffsetTop = offsetTop;
      stableCount = 1;
    }
    if (stableCount >= stableFrames || scheduler.now() - armedAt >= maxWaitMs) {
      settle(height, offsetTop);
      return;
    }
    frameHandle = scheduler.request(onFrame);
  };

  const onEvent = () => {
    if (disposed || frameHandle !== null) return;
    armedAt = scheduler.now();
    lastHeight = NaN;
    lastOffsetTop = NaN;
    stableCount = 0;
    frameHandle = scheduler.request(onFrame);
  };

  source.addEventListener("resize", onEvent);
  source.addEventListener("scroll", onEvent);

  return () => {
    disposed = true;
    source.removeEventListener("resize", onEvent);
    source.removeEventListener("scroll", onEvent);
    if (frameHandle !== null) {
      scheduler.cancel(frameHandle);
      frameHandle = null;
    }
  };
}
