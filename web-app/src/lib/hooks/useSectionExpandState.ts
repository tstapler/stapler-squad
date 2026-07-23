import { useCallback, useState } from "react";

function storageKey(itemId: string, sectionKey: string): string {
  return `backlog-detail-section-${itemId}-${sectionKey}`;
}

/**
 * Per-item, per-section expand/collapse boolean persisted to localStorage
 * (key `backlog-detail-section-${itemId}-${sectionKey}`). Reads/writes are
 * wrapped in try/catch, matching RecentFilesSection.tsx's defensive
 * localStorage pattern — a throwing localStorage (private-browsing quota,
 * disabled storage, etc.) falls back to `defaultExpanded` instead of
 * crashing the component.
 */
export function useSectionExpandState(
  itemId: string,
  sectionKey: string,
  defaultExpanded: boolean
): [boolean, (expanded: boolean) => void] {
  const [expanded, setExpandedState] = useState<boolean>(() => {
    try {
      const stored = localStorage.getItem(storageKey(itemId, sectionKey));
      if (stored === null) return defaultExpanded;
      return stored === "true";
    } catch {
      return defaultExpanded;
    }
  });

  const setExpanded = useCallback(
    (next: boolean) => {
      setExpandedState(next);
      try {
        localStorage.setItem(storageKey(itemId, sectionKey), String(next));
      } catch {
        // Persistence is best-effort only — in-memory state still updates.
      }
    },
    [itemId, sectionKey]
  );

  return [expanded, setExpanded];
}
