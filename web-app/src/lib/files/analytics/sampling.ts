// Volume guards for high-frequency events (ADR-035 section 4). Time is injected so tests use fake clocks.

export interface Clock {
  now(): number;
  setTimeout(fn: () => void, ms: number): unknown;
  clearTimeout(handle: unknown): void;
}

export const realClock: Clock = {
  now: () => Date.now(),
  setTimeout: (fn, ms) => setTimeout(fn, ms),
  clearTimeout: (h) => clearTimeout(h as ReturnType<typeof setTimeout>),
};

export const BUCKET_CAPACITY = 30;
export const BUCKET_WINDOW_MS = 60_000;
export const EXTRA_KEY_WINDOW_MS = 30_000;
/** Events that fire in bursts: only the first per window is kept. */
export const COALESCE_MS: Readonly<Record<string, number>> = {
  "files.tab.switch": 250,
  "files.mobile.keyboard_open": 2000,
  "files.mobile.layout_fallback": 2000,
};

/** True when the previous allowed call for `key` is at least `windowMs` old. */
export function createCoalescer(clock: Clock = realClock) {
  const last = new Map<string, number>();
  return (key: string, windowMs: number): boolean => {
    const now = clock.now();
    const prev = last.get(key);
    if (prev !== undefined && now - prev < windowMs) return false;
    last.set(key, now);
    return true;
  };
}

/** Per-name fixed-window token bucket; drops are counted and reported once when the window closes. */
export function createTokenBucket(onDropped: (key: string, count: number) => void, clock: Clock = realClock) {
  const used = new Map<string, number>();
  const dropped = new Map<string, number>();
  let timer: unknown = null;

  const closeWindow = () => {
    timer = null;
    const report = [...dropped];
    used.clear();
    dropped.clear();
    for (const [key, count] of report) onDropped(key, count);
  };

  return (key: string): boolean => {
    const n = used.get(key) ?? 0;
    timer ??= clock.setTimeout(closeWindow, BUCKET_WINDOW_MS);
    if (n < BUCKET_CAPACITY) {
      used.set(key, n + 1);
      return true;
    }
    dropped.set(key, (dropped.get(key) ?? 0) + 1);
    return false;
  };
}

/** Counts per key and reports each total once per window (or on flush, e.g. `pagehide`). */
export function createWindowAggregator(onClose: (key: string, count: number) => void, clock: Clock = realClock) {
  const counts = new Map<string, number>();
  let timer: unknown = null;

  const flush = () => {
    if (timer !== null) clock.clearTimeout(timer);
    timer = null;
    const report = [...counts];
    counts.clear();
    for (const [key, count] of report) onClose(key, count);
  };

  const add = (key: string) => {
    counts.set(key, (counts.get(key) ?? 0) + 1);
    timer ??= clock.setTimeout(flush, EXTRA_KEY_WINDOW_MS);
  };
  return { add, flush };
}
