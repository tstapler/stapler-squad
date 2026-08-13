"use client";

import { useEffect } from "react";
import { registry, Shortcut } from "./shortcutRegistry";

/**
 * useShortcut — registers a keyboard shortcut on mount, deregisters on unmount.
 *
 * @param id     Unique string identifier for this shortcut (used for deregistration and ? overlay)
 * @param shortcut  Shortcut definition (key, modifiers, label, context, action)
 *
 * The action callback is re-registered whenever it changes (identity comparison),
 * so wrap it in useCallback to avoid unnecessary re-registrations.
 */
export function useShortcut(id: string, shortcut: Shortcut): void {
  useEffect(() => {
    const cleanup = registry.register(id, shortcut);
    return cleanup;
    // Re-register when shortcut identity changes. Callers should useCallback their action.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, shortcut.key, shortcut.modifiers, shortcut.context, shortcut.label, shortcut.action]);
}
