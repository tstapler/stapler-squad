"use client";

import { useEffect, useRef, type RefObject } from "react";
import { matchChord } from "./filesChords";

interface Options {
  searchInputRef: RefObject<HTMLInputElement | null>;
  /** The tree pane is collapsed, so the search box is not usable. */
  collapsed: boolean;
  onQuickOpen: () => void;
  onBack?: () => void;
  onForward?: () => void;
  onGoToLine?: () => void;
  onCloseTab?: () => void;
  /** Ctrl/Cmd+Shift+F: open the Search panel. */
  onSearch?: () => void;
  onNextHit?: () => void;
  onPrevHit?: () => void;
}

/**
 * Files-tab chords (the table and matcher live in filesChords.ts), active only while the tab is visible
 * (`offsetParent` is null under an inactive `display: none` panel). Ctrl/Cmd+F and +P need the tree pane;
 * the rest do not. A chord with no handler is left to the browser.
 */
export function useFilesShortcuts(options: Options): void {
  const live = useRef(options);
  live.current = options;
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const o = live.current;
      const input = o.searchInputRef.current;
      if (!input || input.offsetParent === null) return;
      const chord = matchChord(e);
      if (!chord) return;
      const run: (() => void) | undefined = {
        back: o.onBack,
        forward: o.onForward,
        closeTab: o.onCloseTab,
        gotoLine: o.onGoToLine,
        search: o.onSearch,
        nextHit: o.onNextHit,
        prevHit: o.onPrevHit,
        treeFilter: o.collapsed ? undefined : () => input.focus(),
        quickOpen: o.collapsed ? undefined : o.onQuickOpen,
        save: undefined,
      }[chord.id];
      if (!run) return;
      e.preventDefault();
      run();
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);
}
