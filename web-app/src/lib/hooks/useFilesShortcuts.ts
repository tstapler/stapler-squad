"use client";

import { useEffect, useRef, type RefObject } from "react";

interface Options {
  searchInputRef: RefObject<HTMLInputElement | null>;
  /** The tree pane is collapsed, so the search box is not usable. */
  collapsed: boolean;
  onQuickOpen: () => void;
  onBack?: () => void;
  onForward?: () => void;
  onGoToLine?: () => void;
  onCloseTab?: () => void;
}

/**
 * Files-tab chords, active only while the tab is visible (`offsetParent` is null under an inactive
 * `display: none` panel). Ctrl/Cmd+F and +P need the tree pane; Ctrl/Cmd+G go-to-line, Alt+[ / Alt+]
 * (never Alt+Left/Right, which are the browser's history keys) and Alt+W (never Ctrl+W) do not.
 * Alt chords match the physical key so Option on macOS still works.
 */
export function useFilesShortcuts(options: Options): void {
  const live = useRef(options);
  live.current = options;
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const { searchInputRef, collapsed, onQuickOpen, onBack, onForward, onGoToLine, onCloseTab } = live.current;
      const input = searchInputRef.current;
      if (!input || input.offsetParent === null) return;
      const mod = e.metaKey || e.ctrlKey;
      let run: (() => void) | undefined;
      if (e.altKey && !mod && !e.shiftKey) {
        if (e.code === "BracketLeft") run = onBack;
        else if (e.code === "BracketRight") run = onForward;
        else if (e.code === "KeyW") run = onCloseTab;
      } else if (mod && !e.altKey && !e.shiftKey) {
        if (e.key === "g") run = onGoToLine;
        else if (!collapsed && e.key === "f") run = () => input.focus();
        else if (!collapsed && e.key === "p") run = onQuickOpen;
      }
      if (!run) return;
      e.preventDefault();
      run();
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);
}
