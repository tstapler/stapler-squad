"use client";

import { useSyncExternalStore } from "react";

const KEY = "ssq.files.fontPx";
const CHANGE = "ssq-files-font-change";

export const DESKTOP_DEFAULT_PX = 13;
/** Soft keyboards zoom the page when a focused field renders below 16px. */
export const COARSE_MIN_PX = 16;
export const DESKTOP_MIN_PX = 8;
export const MAX_PX = 28;

export const defaultFontPx = (coarse: boolean) => (coarse ? COARSE_MIN_PX : DESKTOP_DEFAULT_PX);
export const clampFontPx = (px: number, coarse: boolean) => Math.min(MAX_PX, Math.max(coarse ? COARSE_MIN_PX : DESKTOP_MIN_PX, Math.round(px)));

/** Set only while storage rejects writes, so the step still applies for this session. */
let unsavedPx: number | null = null;

/** The stepped size for this device, or the default; never below the touch floor on a coarse pointer. */
export function readFontPx(coarse: boolean): number {
  if (unsavedPx !== null) return clampFontPx(unsavedPx, coarse);
  try {
    const stored = Number(window.localStorage.getItem(KEY));
    if (Number.isFinite(stored) && stored > 0) return clampFontPx(stored, coarse);
  } catch {
    // storage unavailable: default
  }
  return defaultFontPx(coarse);
}

/** One 1px step (A+ / A-), persisted per device. Returns the new size. */
export function stepFontPx(delta: 1 | -1, coarse: boolean): number {
  const next = clampFontPx(readFontPx(coarse) + delta, coarse);
  try {
    window.localStorage.setItem(KEY, String(next));
    unsavedPx = null;
  } catch {
    unsavedPx = next;
  }
  window.dispatchEvent(new Event(CHANGE));
  return next;
}

const subscribe = (cb: () => void) => {
  window.addEventListener(CHANGE, cb);
  window.addEventListener("storage", cb);
  return () => {
    window.removeEventListener(CHANGE, cb);
    window.removeEventListener("storage", cb);
  };
};

export function useFontPx(coarse: boolean): number {
  return useSyncExternalStore(subscribe, () => readFontPx(coarse), () => defaultFontPx(coarse));
}
