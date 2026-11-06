/**
 * Persisted scroll settings (design/ux.md S6): the drag-routing override and the
 * Gesture scrolling switch. Per device, not per session. Storage can throw
 * (private mode, quota), so every access falls back to defaults and the
 * in-memory choice keeps applying for the rest of the session.
 */
import { mobileDebug } from "./mobileDebug";
import type { ScrollOverride } from "./scrollRouting";

export const SCROLL_OVERRIDE_KEY = "terminal-scroll-override";
export const GESTURE_SCROLL_KEY = "terminal-gesture-scroll";

export const DEFAULT_SCROLL_OVERRIDE: ScrollOverride = "auto";
export const DEFAULT_GESTURE_SCROLL = true;

type StorageLike = Pick<Storage, "getItem" | "setItem" | "removeItem">;

export interface ScrollSettingsStore {
  getOverride(): ScrollOverride;
  setOverride(value: ScrollOverride): void;
  getGestureScroll(): boolean;
  setGestureScroll(on: boolean): void;
  /** Fires after any change; returns the unsubscribe function. */
  subscribe(listener: () => void): () => void;
}

export interface ScrollSettingsOptions {
  /** Resolved on every access so a throwing or missing storage is tolerated. */
  getStorage?: () => StorageLike | null;
  onOverrideChange?: (from: ScrollOverride, to: ScrollOverride) => void;
}

function defaultStorage(): StorageLike | null {
  return typeof localStorage === "undefined" ? null : localStorage;
}

function parseOverride(raw: string | null | undefined): ScrollOverride {
  return raw === "local" || raw === "tui" ? raw : DEFAULT_SCROLL_OVERRIDE;
}

export function createScrollSettings(options: ScrollSettingsOptions = {}): ScrollSettingsStore {
  const getStorage = options.getStorage ?? defaultStorage;
  const onOverrideChange =
    options.onOverrideChange ?? ((from, to) => mobileDebug.overrideChange(from, to));
  const listeners = new Set<() => void>();

  // Set once the user changes a value, so it wins over a storage that rejected the write.
  let memoryOverride: ScrollOverride | null = null;
  let memoryGesture: boolean | null = null;

  const readRaw = (key: string): string | null => {
    try {
      return getStorage()?.getItem(key) ?? null;
    } catch {
      return null;
    }
  };

  const write = (apply: (storage: StorageLike) => void): void => {
    try {
      const storage = getStorage();
      if (storage) apply(storage);
    } catch {
      // keep the in-memory choice
    }
  };

  const notify = () => listeners.forEach((l) => l());

  const getOverride = (): ScrollOverride => memoryOverride ?? parseOverride(readRaw(SCROLL_OVERRIDE_KEY));
  const getGestureScroll = (): boolean =>
    memoryGesture ?? (readRaw(GESTURE_SCROLL_KEY) === "off" ? false : DEFAULT_GESTURE_SCROLL);

  return {
    getOverride,
    getGestureScroll,
    setOverride(value) {
      const previous = getOverride();
      memoryOverride = value;
      write((s) => (value === "auto" ? s.removeItem(SCROLL_OVERRIDE_KEY) : s.setItem(SCROLL_OVERRIDE_KEY, value)));
      if (previous === value) return;
      onOverrideChange(previous, value);
      notify();
    },
    setGestureScroll(on) {
      const previous = getGestureScroll();
      memoryGesture = on;
      write((s) => (on ? s.removeItem(GESTURE_SCROLL_KEY) : s.setItem(GESTURE_SCROLL_KEY, "off")));
      if (previous !== on) notify();
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
}

/** Shared store for app code. */
export const scrollSettings: ScrollSettingsStore = createScrollSettings();
