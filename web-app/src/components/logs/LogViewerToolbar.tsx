import React, { useState, useCallback } from "react";
import { LevelFilterChips } from "./LevelFilterChips";
import {
  toolbar,
  toolbarRow,
  searchWrapper,
  searchInput,
  matchCounter,
  searchIconButton,
  searchExpandableRow,
  searchExpandableRowOpen,
  searchDoneButton,
  liveTailButton,
  liveTailDot,
} from "./LogViewerToolbar.css";

interface LogViewerToolbarProps {
  searchQuery: string;
  onSearchChange: (q: string) => void;
  matchCount: number;
  totalCount: number;
  levelFilters: string[];
  onLevelFiltersChange: (l: string[]) => void;
  /** Ref forwarded from LogViewer so keyboard shortcut '/' can focus the input */
  searchInputRef?: React.RefObject<HTMLInputElement | null>;
  liveTailEnabled?: boolean;
  isFollowing?: boolean;
  onLiveTailChange?: (enabled: boolean) => void;
}

/**
 * LogViewerToolbar — search input with match counter, level filter chips,
 * and optional live-tail toggle.
 *
 * Epic 3: searchInputRef wired for '/' keyboard shortcut; match counter display.
 * Epic 5 / T3: Collapsible search bar on narrow screens (< 430px).
 *   - Wide screens (≥ 431px): search input always visible in the chips row.
 *   - Narrow screens (< 430px): shows a 🔍 icon button; tapping it expands a
 *     full-width search input row below the chips. A "Done" button collapses it
 *     (does NOT clear the search query).
 */
export function LogViewerToolbar({
  searchQuery,
  onSearchChange,
  matchCount,
  totalCount,
  levelFilters,
  onLevelFiltersChange,
  searchInputRef,
  liveTailEnabled = false,
  isFollowing = false,
  onLiveTailChange,
}: LogViewerToolbarProps) {
  // T3: narrow-screen expanded state
  const [isSearchExpanded, setIsSearchExpanded] = useState(false);

  const handleSearchIconClick = useCallback(() => {
    setIsSearchExpanded(true);
    // autoFocus via setTimeout to ensure the row is visible first
    setTimeout(() => searchInputRef?.current?.focus(), 50);
  }, [searchInputRef]);

  const handleDone = useCallback(() => {
    setIsSearchExpanded(false);
    searchInputRef?.current?.blur();
  }, [searchInputRef]);

  // Combined class for the expandable search row: always open on wide screens,
  // open on narrow only when isSearchExpanded is true.
  const expandableRowClass = isSearchExpanded
    ? `${searchExpandableRow} ${searchExpandableRowOpen}`
    : searchExpandableRow;

  const searchInputEl = (
    <div className={searchWrapper}>
      <input
        ref={searchInputRef}
        type="search"
        inputMode="search"
        autoCapitalize="none"
        autoCorrect="off"
        autoComplete="off"
        value={searchQuery}
        onChange={(e) => onSearchChange(e.target.value)}
        placeholder="Search logs... (/ to focus)"
        aria-label="Search logs"
        className={searchInput}
        style={searchQuery ? { paddingRight: 80 } : undefined}
      />
      {searchQuery && (
        <span aria-live="polite" className={matchCounter}>
          {matchCount} / {totalCount}
        </span>
      )}
    </div>
  );

  return (
    <div className={toolbar}>
      {/* Primary row: live-tail toggle + chips + search icon (narrow) + search input (wide) */}
      <div className={toolbarRow}>
        {/* Live-tail toggle — always visible; green dot when live and following */}
        <button
          type="button"
          className={liveTailButton}
          aria-label="Toggle live tail"
          aria-pressed={liveTailEnabled}
          data-testid="live-tail-toggle"
          data-live={liveTailEnabled && isFollowing ? "true" : "false"}
          onClick={() => onLiveTailChange?.(!liveTailEnabled)}
        >
          <span className={liveTailDot} />
          {liveTailEnabled && isFollowing ? "Live" : "Paused"}
        </button>

        {/* T3: search icon button — visible only on narrow screens via CSS */}
        <button
          type="button"
          className={searchIconButton}
          aria-label="Expand search"
          onClick={handleSearchIconClick}
        >
          🔍
        </button>

        <LevelFilterChips active={levelFilters} onChange={onLevelFiltersChange} />

        {/*
          T3: On wide screens (≥ 431px) this row is always display:flex.
          On narrow screens it is display:none — the expandable row below
          provides the search input when expanded.
        */}
        <div className={searchExpandableRow} style={{ flex: 1 }}>
          {searchInputEl}
        </div>
      </div>

      {/*
        T3: Expanded search row — shown below the chips row on narrow screens
        when isSearchExpanded is true. Hidden on wide screens via CSS.
      */}
      {isSearchExpanded && (
        <div className={`${toolbarRow} ${expandableRowClass}`}>
          {searchInputEl}
          <button
            type="button"
            className={searchDoneButton}
            aria-label="Collapse search"
            onClick={handleDone}
          >
            Done
          </button>
        </div>
      )}
    </div>
  );
}
