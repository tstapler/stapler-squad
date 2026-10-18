"use client";

import { useSyncExternalStore } from "react";

const KEY = "ssq.files.search.largeFiles";
const CHANGE = "ssq-files-large-files-change";

/** Per-device "Include large files" search toggle; off until the user turns it on. */
export function readLargeFiles(): boolean {
  try {
    return window.localStorage.getItem(KEY) === "1";
  } catch {
    return false; // storage unavailable: default
  }
}

export function writeLargeFiles(on: boolean): void {
  try {
    window.localStorage.setItem(KEY, on ? "1" : "0");
  } catch {
    // not persisted; the event below still applies it for this session
  }
  window.dispatchEvent(new Event(CHANGE));
}

const subscribe = (cb: () => void) => {
  window.addEventListener(CHANGE, cb);
  window.addEventListener("storage", cb);
  return () => {
    window.removeEventListener(CHANGE, cb);
    window.removeEventListener("storage", cb);
  };
};

export const useLargeFiles = (): boolean => useSyncExternalStore(subscribe, readLargeFiles, () => false);
