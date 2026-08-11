"use client";

import { useState, useCallback, useEffect } from "react";

const STORAGE_KEY = "cockpit.listColumnWidth";
const DEFAULT_WIDTH = 320;
const MIN_WIDTH = 240;

/**
 * useListColumnWidth — persists the session list column width in localStorage.
 * Returns [width, setWidth]. setWidth clamps to [MIN_WIDTH, 50% viewport] and saves.
 */
export function useListColumnWidth(): [number, (w: number) => void] {
  const [width, setWidthState] = useState<number>(() => {
    if (typeof localStorage === "undefined") return DEFAULT_WIDTH;
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        const parsed = parseInt(stored, 10);
        if (!isNaN(parsed) && parsed >= MIN_WIDTH) return parsed;
      }
    } catch {
      // ignore
    }
    return DEFAULT_WIDTH;
  });

  const setWidth = useCallback((w: number) => {
    const maxWidth = typeof window !== "undefined" ? window.innerWidth * 0.5 : 800;
    const clamped = Math.max(MIN_WIDTH, Math.min(maxWidth, w));
    setWidthState(clamped);
    try {
      localStorage.setItem(STORAGE_KEY, String(clamped));
    } catch {
      // ignore
    }
  }, []);

  // Re-read from localStorage when the component first mounts on the client
  useEffect(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        const parsed = parseInt(stored, 10);
        if (!isNaN(parsed) && parsed >= MIN_WIDTH) {
          setWidthState(parsed);
        }
      }
    } catch {
      // ignore
    }
  }, []);

  return [width, setWidth];
}
