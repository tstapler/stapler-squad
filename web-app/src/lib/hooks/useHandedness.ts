"use client";

import { useState, useEffect } from "react";

export const HANDEDNESS_KEY = "stapler-squad:left-handed";

/**
 * Reads and persists the user's handedness preference.
 *
 * On mount (and on every change), sets `data-left-handed` on <html> so that
 * CSS can flip thumb-sensitive layouts with a single selector:
 *
 *   selectors: { ':root[data-left-handed] &': { flexDirection: 'row' } }
 *
 * Components that need to respond in JS can read `leftHanded` directly.
 */
export function useHandedness() {
  const [leftHanded, setLeftHanded] = useState(false);

  // Read preference on mount (client-side only)
  useEffect(() => {
    try {
      const stored = localStorage.getItem(HANDEDNESS_KEY) === "true";
      setLeftHanded(stored);
      applyToRoot(stored);
    } catch { /* ignore SSR / private browsing */ }
  }, []);

  const toggleHandedness = () => {
    const next = !leftHanded;
    setLeftHanded(next);
    try { localStorage.setItem(HANDEDNESS_KEY, String(next)); } catch { /* ignore */ }
    applyToRoot(next);
  };

  return { leftHanded, toggleHandedness };
}

function applyToRoot(leftHanded: boolean) {
  if (typeof document === "undefined") return;
  if (leftHanded) {
    document.documentElement.dataset.leftHanded = "true";
  } else {
    delete document.documentElement.dataset.leftHanded;
  }
}
