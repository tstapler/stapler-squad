"use client";

import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { FileTree } from "./FileTree";
import type { FileTreeHandle, SortMode } from "./FileTree";
import { FileContentViewer } from "./FileContentViewer";
import { useSessionVcsContext } from "@/lib/contexts/SessionVcsContext";
import { useResizablePanel } from "@/lib/hooks/useResizablePanel";
import { buildGitStatusMap, buildLineStatsMap } from "@/lib/utils/gitStatus";
import { TreeResizeHandle } from "./TreeResizeHandle";
import { RecentFilesSection } from "./RecentFilesSection";
import { QuickOpenPalette } from "./QuickOpenPalette";
import {
  container, treePane, treePaneCollapsed, contentPane, toolbar, searchInput,
  toolbarLabel, toolbarButton, searchCount, treeWrapper,
  mobilePaneHidden, mobilePaneVisible, mobileBackButton,
  toolbarButtonMobileHidden, mobileSearchButton, toolbarDivider,
} from "./FilesTab.css";

// ---- Props ----

interface FilesTabProps {
  sessionId: string;
  baseUrl: string;
  /** Path to pre-select when the tab opens (e.g. from VCS panel cross-link). */
  initialSelectedPath?: string | null;
  onSelectedPathChange?: (path: string | null) => void;
}

// ---- Component ----

export function FilesTab({
  sessionId,
  baseUrl,
  initialSelectedPath,
  onSelectedPathChange,
}: FilesTabProps) {
  const [selectedPath, setSelectedPath] = useState<string | null>(initialSelectedPath ?? null);
  const [includeIgnored, setIncludeIgnored] = useState(false);
  const [sortBy, setSortBy] = useState<SortMode>("name");
  const [filterChangedOnly, setFilterChangedOnly] = useState(false);
  const [searchTerm, setSearchTerm] = useState("");
  const [searchResultCount, setSearchResultCount] = useState<number | null>(null);
  const [searchResultTruncated, setSearchResultTruncated] = useState(false);
  const [mobilePane, setMobilePane] = useState<"tree" | "content">("tree");
  const [recentPaths, setRecentPaths] = useState<string[]>([]);
  const [isQuickOpenOpen, setIsQuickOpenOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const fileTreeRef = useRef<FileTreeHandle>(null);

  // Resizable panel
  const panel = useResizablePanel({
    storageKey: "filestab.treeWidth",
    defaultWidth: 260,
    minWidth: 160,
    maxWidthFraction: 0.5,
  });

  // VCS status/diff come from shared context — no independent fetch.
  const { status, diff, statusLoading: vcsLoading, refreshStatus } = useSessionVcsContext();

  // Derive git status + per-file line-count maps from shared VCS status.
  const changedFiles = useMemo(() => {
    if (!status) return [];
    const { stagedFiles, unstagedFiles, untrackedFiles } = status;
    return [...stagedFiles, ...unstagedFiles, ...untrackedFiles];
  }, [status]);
  const gitStatusMap = useMemo(() => buildGitStatusMap(changedFiles), [changedFiles]);
  const lineStatsMap = useMemo(() => buildLineStatsMap(changedFiles), [changedFiles]);

  // Notify parent when selection changes.
  const handleFileSelect = useCallback(
    (path: string) => {
      setSelectedPath(path);
      onSelectedPathChange?.(path);
      setMobilePane("content");
      setRecentPaths((prev) => [path, ...prev.filter((p) => p !== path)].slice(0, 8));
    },
    [onSelectedPathChange]
  );

  // Apply initialSelectedPath changes from parent (VCS cross-link).
  useEffect(() => {
    if (initialSelectedPath !== undefined && initialSelectedPath !== selectedPath) {
      setSelectedPath(initialSelectedPath);
      if (initialSelectedPath) {
        fileTreeRef.current?.revealPath(initialSelectedPath);
      }
    }
  }, [initialSelectedPath]); // eslint-disable-line react-hooks/exhaustive-deps

  // Cmd+F / Ctrl+F focuses the search input.
  // Cmd+P / Ctrl+P opens quick open palette.
  // Guard: only intercept when the files tab is actually visible (offsetParent is null when
  // an ancestor has display:none, which happens when the tab panel is inactive).
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "f") {
        if (!searchInputRef.current) return;
        if (searchInputRef.current.offsetParent === null) return;
        if (panel.collapsed) return;
        e.preventDefault();
        searchInputRef.current.focus();
      }
      if ((e.metaKey || e.ctrlKey) && e.key === "p") {
        if (!searchInputRef.current || searchInputRef.current.offsetParent === null) return;
        if (panel.collapsed) return;
        e.preventDefault();
        setIsQuickOpenOpen(true);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [panel.collapsed]);

  // Build tree pane class names
  const treePaneClasses = [
    treePane,
    panel.collapsed ? treePaneCollapsed : "",
    mobilePane === "content" ? mobilePaneHidden : "",
  ]
    .filter(Boolean)
    .join(" ");

  // Build content pane class names
  const contentPaneClasses = [
    contentPane,
    mobilePane === "tree" ? mobilePaneHidden : "",
    mobilePane === "content" ? mobilePaneVisible : "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <div className={container} ref={panel.containerRef}>
      {/* Left pane: file tree */}
      <div
        className={treePaneClasses}
        style={{ width: panel.collapsed ? 0 : panel.width }}
      >
        <div className={toolbar}>
          <input
            ref={searchInputRef}
            type="search"
            className={searchInput}
            placeholder="Search files…"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                setSearchTerm("");
                searchInputRef.current?.blur();
              }
            }}
            enterKeyHint="search"
            aria-label="Search files"
          />
          {searchResultCount !== null && searchTerm.length >= 2 && (
            <span
              className={searchCount}
              title={searchResultTruncated ? "Results truncated at 500" : undefined}
              aria-live="polite"
              aria-atomic="true"
            >
              {searchResultCount}{searchResultTruncated ? "+" : ""} match{searchResultCount !== 1 ? "es" : ""}
            </span>
          )}
          <label className={toolbarLabel} title="Show gitignored files">
            <input
              type="checkbox"
              checked={includeIgnored}
              onChange={(e) => setIncludeIgnored(e.target.checked)}
            />
            Ignored
          </label>
          <label className={toolbarLabel} title="Show only files with git changes">
            <input
              type="checkbox"
              checked={filterChangedOnly}
              onChange={(e) => setFilterChangedOnly(e.target.checked)}
            />
            Changed
          </label>
          <button
            className={`${toolbarButton} ${toolbarButtonMobileHidden}`}
            onClick={() => setSortBy((prev) => (prev === "name" ? "type" : "name"))}
            title={`Sort by ${sortBy === "name" ? "type" : "name"}`}
            aria-label={`Sort by ${sortBy === "name" ? "type" : "name"}`}
          >
            Sort: {sortBy === "name" ? "Name" : "Type"}
          </button>
          <button
            className={`${toolbarButton} ${toolbarButtonMobileHidden}`}
            onClick={() => fileTreeRef.current?.collapseAll()}
            title="Collapse all directories"
            aria-label="Collapse all directories"
          >
            ⊟
          </button>
          <div className={toolbarDivider} />
          {panel.collapsed ? (
            <button
              className={`${toolbarButton} ${toolbarButtonMobileHidden}`}
              onClick={() => panel.expand()}
              title="Expand file tree panel"
              aria-label="Expand file tree panel"
            >
              ⊞
            </button>
          ) : (
            <button
              className={`${toolbarButton} ${toolbarButtonMobileHidden}`}
              onClick={() => panel.collapse()}
              title="Collapse file tree panel"
              aria-label="Collapse file tree panel"
            >
              ⊠
            </button>
          )}
          <button
            className={toolbarButton}
            onClick={() => refreshStatus()}
            title="Refresh git status"
            aria-label="Refresh git status"
            disabled={vcsLoading}
          >
            {vcsLoading ? "⟳" : "↺"}
          </button>
        </div>
        <RecentFilesSection
          paths={recentPaths}
          selectedPath={selectedPath}
          onSelect={handleFileSelect}
        />
        <div className={treeWrapper}>
          <FileTree
            ref={fileTreeRef}
            sessionId={sessionId}
            baseUrl={baseUrl}
            onFileSelect={handleFileSelect}
            gitStatusMap={gitStatusMap}
            lineStatsMap={lineStatsMap}
            sortBy={sortBy}
            filterChangedOnly={filterChangedOnly}
            selectedPath={selectedPath}
            includeIgnored={includeIgnored}
            searchTerm={searchTerm}
            onSearchResults={(count, truncated) => {
              setSearchResultCount(count);
              setSearchResultTruncated(truncated);
            }}
          />
        </div>
      </div>

      {!panel.collapsed && <TreeResizeHandle {...panel.handleProps} />}

      {/* Right pane: file content */}
      <div className={contentPaneClasses}>
        <button
          className={mobileBackButton}
          onClick={() => setMobilePane("tree")}
        >
          ← Files
        </button>
        <button
          className={mobileSearchButton}
          onClick={() => setIsQuickOpenOpen(true)}
          aria-label="Search files"
        >
          🔍
        </button>
        <FileContentViewer
          sessionId={sessionId}
          filePath={selectedPath}
          baseUrl={baseUrl}
          diffContent={diff?.content}
        />
      </div>

      {isQuickOpenOpen && (
        <QuickOpenPalette
          sessionId={sessionId}
          baseUrl={baseUrl}
          recentPaths={recentPaths}
          onSelect={(path) => {
            setIsQuickOpenOpen(false);
            handleFileSelect(path);
            fileTreeRef.current?.revealPath(path);
          }}
          onClose={() => setIsQuickOpenOpen(false)}
        />
      )}
    </div>
  );
}
