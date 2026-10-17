"use client";

import { useEffect, type RefObject } from "react";

interface Options {
  searchInputRef: RefObject<HTMLInputElement | null>;
  /** The tree pane is collapsed, so the search box is not usable. */
  collapsed: boolean;
  onQuickOpen: () => void;
}

/**
 * Cmd/Ctrl+F focuses the search box and Cmd/Ctrl+P opens quick open, but only while the Files tab is
 * visible (`offsetParent` is null under an inactive `display: none` tab panel).
 */
export function useFilesShortcuts({ searchInputRef, collapsed, onQuickOpen }: Options): void {
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || (e.key !== "f" && e.key !== "p")) return;
      const input = searchInputRef.current;
      if (!input || input.offsetParent === null || collapsed) return;
      e.preventDefault();
      if (e.key === "f") input.focus();
      else onQuickOpen();
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [searchInputRef, collapsed, onQuickOpen]);
}
