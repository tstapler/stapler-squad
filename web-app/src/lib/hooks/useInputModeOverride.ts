"use client";

import { useState } from "react";

export const INPUT_MODE_OVERRIDE_KEY = "stapler-squad:input-mode-override";

export type InputModeOverride = "auto" | "desktop" | "touch";

/**
 * Reads and persists the user's override for mouse+keyboard-on-mobile detection.
 *
 * "auto" (default) lets the terminal toolbar decide based on live hardware
 * signals (matchMedia hover/pointer:fine via useViewport().hasFinePointer).
 * "desktop" and "touch" force that decision regardless of what the hardware
 * reports, for the rare case detection gets it wrong (e.g. some Android
 * keyboard cases, or a user who prefers the expanded toolbar despite a mouse).
 */
export function useInputModeOverride() {
  // Read synchronously via a lazy initializer (not a mount effect) — a caller that
  // derives a one-time default from inputModeOverride on its own mount effect would
  // otherwise see the "auto" fallback before an async localStorage-read effect here
  // resolved the persisted value, and could apply the wrong default irreversibly.
  const [inputModeOverride, setInputModeOverrideState] = useState<InputModeOverride>(() => {
    if (typeof localStorage === "undefined") return "auto";
    try {
      const stored = localStorage.getItem(INPUT_MODE_OVERRIDE_KEY);
      if (stored === "auto" || stored === "desktop" || stored === "touch") {
        return stored;
      }
    } catch { /* ignore SSR / private browsing */ }
    return "auto";
  });

  const setInputModeOverride = (next: InputModeOverride) => {
    setInputModeOverrideState(next);
    try { localStorage.setItem(INPUT_MODE_OVERRIDE_KEY, next); } catch { /* ignore */ }
  };

  return { inputModeOverride, setInputModeOverride };
}
