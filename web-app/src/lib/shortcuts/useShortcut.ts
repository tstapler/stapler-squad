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
 *
 * Pass `enabled: false` from a component that can be mounted many times at once (one per pane)
 * so only one instance owns the id; otherwise each mount overwrites the last and the first
 * unmount deletes the survivors' registration.
 */
export function useShortcut(id: string, shortcut: Shortcut, enabled = true): void {
  useEffect(() => {
    if (!enabled) return;
    const cleanup = registry.register(id, shortcut);
    return cleanup;
    // Re-register when shortcut identity changes. Callers should useCallback their action.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    id,
    enabled,
    shortcut.key,
    shortcut.code,
    shortcut.modifiers,
    shortcut.context,
    shortcut.label,
    shortcut.action,
    shortcut.sequence?.join("\u0000"),
    shortcut.group,
    shortcut.hidden,
  ]);
}
