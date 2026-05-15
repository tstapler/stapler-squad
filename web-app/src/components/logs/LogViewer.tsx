"use client";

import { useEffect, useRef, useState, useCallback } from "react";
import { useLogViewer } from "@/lib/hooks/useLogViewer";
import { VirtualLogList } from "./VirtualLogList";
import { JumpToLatestButton } from "./JumpToLatestButton";
import { LogViewerToolbar } from "./LogViewerToolbar";
import { ShortcutHelpOverlay } from "./ShortcutHelpOverlay";
import * as styles from "./LogViewer.css";

interface LogViewerProps {
  source: "app" | "session";
  sessionId?: string;
}

/**
 * LogViewer — shared log display component for both the app logs page and session logs tab.
 * Epic 2: wired with react-virtuoso virtual list, live-tail follow/pause state machine,
 * and Jump to Latest pill.
 * Epic 3: keyboard shortcuts (/, Esc, g, G, =, ?, Cmd+F) and shortcut help overlay.
 */
export function LogViewer({ source, sessionId }: LogViewerProps) {
  const {
    logs,
    isFollowing,
    searchQuery,
    setSearchQuery,
    levelFilters,
    setLevelFilters,
    matchCount,
    totalCount,
    toggleRow,
    expandedRowIndex,
    selectedRowIndex,
    setSelectedRowIndex,
    jumpToLatest,
    queuedNewLineCount,
    onAtBottomStateChange,
    virtuosoRef,
    liveTailEnabled,
    setLiveTailEnabled,
  } = useLogViewer(source, sessionId);

  const containerRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const [showShortcutHelp, setShowShortcutHelp] = useState(false);

  // T9: Keyboard shortcuts — liveTailEnabled in a ref to avoid stale closure
  const liveTailEnabledRef = useRef(liveTailEnabled);
  useEffect(() => {
    liveTailEnabledRef.current = liveTailEnabled;
  }, [liveTailEnabled]);

  // selectedRowIndex in a ref to avoid stale closure in keyboard handler
  const selectedRowIndexRef = useRef(selectedRowIndex);
  useEffect(() => {
    selectedRowIndexRef.current = selectedRowIndex;
  }, [selectedRowIndex]);

  // logs length in a ref to avoid stale closure
  const logsLengthRef = useRef(logs.length);
  useEffect(() => {
    logsLengthRef.current = logs.length;
  }, [logs.length]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      // Do not fire shortcuts when focus is inside an input/textarea/select
      const target = e.target as HTMLElement;
      if (["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)) return;

      // Cmd+F / Ctrl+F — intercept browser find and focus search
      if (e.key === "f" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        searchInputRef.current?.focus();
        return;
      }

      switch (e.key) {
        case "/":
          e.preventDefault();
          searchInputRef.current?.focus();
          break;
        case "Escape":
          searchInputRef.current?.blur();
          setSearchQuery("");
          break;
        case "g":
          virtuosoRef.current?.scrollToIndex(0);
          break;
        case "G":
          virtuosoRef.current?.scrollToIndex({ index: "LAST", behavior: "smooth" });
          break;
        case "=":
          setLiveTailEnabled(!liveTailEnabledRef.current);
          break;
        case "?":
          setShowShortcutHelp((prev) => !prev);
          break;
        case "j":
        case "ArrowDown": {
          e.preventDefault();
          const len = logsLengthRef.current;
          if (len === 0) break;
          const cur = selectedRowIndexRef.current;
          const next = cur === null ? 0 : Math.min(cur + 1, len - 1);
          setSelectedRowIndex(next);
          virtuosoRef.current?.scrollToIndex({ index: next, behavior: "smooth" });
          break;
        }
        case "k":
        case "ArrowUp": {
          e.preventDefault();
          const len = logsLengthRef.current;
          if (len === 0) break;
          const cur = selectedRowIndexRef.current;
          const prev = cur === null ? 0 : Math.max(cur - 1, 0);
          setSelectedRowIndex(prev);
          virtuosoRef.current?.scrollToIndex({ index: prev, behavior: "smooth" });
          break;
        }
        default:
          break;
      }
    },
    [setSearchQuery, virtuosoRef, setLiveTailEnabled, setSelectedRowIndex],
  );

  // T6: VisualViewport resize handler — recalculates scroll container height
  // when the iOS software keyboard appears or disappears. Uses
  // window.visualViewport (not window.resize) to get the actual visible height.
  // Debounced 400ms to avoid thrashing during keyboard animation.
  useEffect(() => {
    if (!window.visualViewport) return;
    let debounceTimer: ReturnType<typeof setTimeout>;
    const handler = () => {
      clearTimeout(debounceTimer);
      debounceTimer = setTimeout(() => {
        const height = window.visualViewport!.height;
        containerRef.current?.style.setProperty("--log-container-height", `${height}px`);
      }, 400);
    };
    window.visualViewport.addEventListener("resize", handler);
    return () => {
      window.visualViewport!.removeEventListener("resize", handler);
      clearTimeout(debounceTimer);
    };
  }, []);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const onKeyDown = (e: KeyboardEvent) => {
      // Cmd+F: intercept at window level when our container is active
      if (e.key === "f" && (e.metaKey || e.ctrlKey)) {
        const active = document.activeElement;
        if (active && !container.contains(active) && active !== document.body) return;
        handleKeyDown(e);
        return;
      }
      // Other shortcuts: only fire when focus is inside our container or on body
      const active = document.activeElement;
      if (container.contains(active) || active === document.body) {
        handleKeyDown(e);
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [handleKeyDown]);

  return (
    <div
      className={source === "session" ? styles.containerSession : styles.container}
      ref={containerRef}
      tabIndex={-1}
      data-testid="log-viewer"
    >
      <LogViewerToolbar
        searchQuery={searchQuery}
        onSearchChange={setSearchQuery}
        matchCount={matchCount}
        totalCount={totalCount}
        levelFilters={levelFilters}
        onLevelFiltersChange={setLevelFilters}
        searchInputRef={searchInputRef}
        liveTailEnabled={liveTailEnabled}
        isFollowing={isFollowing}
        onLiveTailChange={setLiveTailEnabled}
      />

      {/* Scroll region — flex: 1 so it fills the remaining height */}
      <div style={{ flex: 1, minHeight: 0, position: "relative" }}>
        <VirtualLogList
          entries={logs}
          isFollowing={isFollowing}
          expandedRowIndex={expandedRowIndex}
          selectedRowIndex={selectedRowIndex}
          searchQuery={searchQuery}
          onAtBottomStateChange={onAtBottomStateChange}
          onToggleRow={toggleRow}
          virtuosoRef={virtuosoRef}
        />

        <JumpToLatestButton
          newLineCount={queuedNewLineCount}
          onClick={jumpToLatest}
        />
      </div>

      {/* Shortcut help overlay is app-surface-only — session tab is too compact */}
      {source === "app" && (
        <ShortcutHelpOverlay
          isOpen={showShortcutHelp}
          onClose={() => setShowShortcutHelp(false)}
        />
      )}
    </div>
  );
}

export default LogViewer;
