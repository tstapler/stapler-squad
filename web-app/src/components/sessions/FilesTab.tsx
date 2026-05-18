"use client";

import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { FileStatus, FileChange } from "@/gen/session/v1/types_pb";
import { FileTree } from "./FileTree";
import type { FileTreeHandle } from "./FileTree";
import { FileContentViewer } from "./FileContentViewer";
import { useSessionVcsContext } from "@/lib/contexts/SessionVcsContext";
import { useResizablePanel } from "@/lib/hooks/useResizablePanel";
import { TreeResizeHandle } from "./TreeResizeHandle";
import { RecentFilesSection } from "./RecentFilesSection";
import { QuickOpenPalette } from "./QuickOpenPalette";
import {
  container, treePane, treePaneCollapsed, contentPane, toolbar, searchInput,
  toolbarLabel, toolbarButton, searchCount, treeWrapper,
  mobilePaneHidden, mobilePaneVisible, mobileBackButton,
} from "./FilesTab.css";

// ---- Git status helpers ----

function fileChangeToStatusLetter(status: FileStatus): string {
  switch (status) {
    case FileStatus.MODIFIED:    return "M";
    case FileStatus.ADDED:       return "A";
    case FileStatus.DELETED:     return "D";
    case FileStatus.RENAMED:     return "R";
    case FileStatus.UNTRACKED:   return "?";
    case FileStatus.CONFLICT:    return "U";
    default:                     return "";
  }
}

function buildGitStatusMap(files: FileChange[]): Map<string, string> {
  const map = new Map<string, string>();
  for (const f of files) {
    const letter = fileChangeToStatusLetter(f.status);
    if (letter && f.path) {
      map.set(f.path, letter);
    }
  }
  return map;
}

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

  // VCS status comes from shared context — no independent fetch.
  const { status, statusLoading: vcsLoading, refreshStatus } = useSessionVcsContext();

  // Derive git status map from shared VCS status.
  const gitStatusMap = useMemo(() => {
    if (!status) return new Map<string, string>();
    const { stagedFiles, unstagedFiles, untrackedFiles } = status;
    return buildGitStatusMap([...stagedFiles, ...unstagedFiles, ...untrackedFiles]);
  }, [status]);

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
        e.preventDefault();
        searchInputRef.current.focus();
      }
      if ((e.metaKey || e.ctrlKey) && e.key === "p") {
        if (!searchInputRef.current || searchInputRef.current.offsetParent === null) return;
        e.preventDefault();
        setIsQuickOpenOpen(true);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

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
        style={{ width: (panel.collapsed || mobilePane === 'content') ? 0 : panel.width }}
      >
        <div className={toolbar}>
          <input
            ref={searchInputRef}
            type="search"
            className={searchInput}
            placeholder="Search files… (⌘F)"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Escape") {
                setSearchTerm("");
                searchInputRef.current?.blur();
              }
            }}
            aria-label="Search files"
          />
          {searchResultCount !== null && searchTerm.length >= 2 && (
            <span className={searchCount} title={searchResultTruncated ? "Results truncated at 500" : undefined}>
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
          <button
            className={toolbarButton}
            onClick={() => fileTreeRef.current?.collapseAll()}
            title="Collapse all directories"
          >
            ⊟
          </button>
          {panel.collapsed ? (
            <button
              className={toolbarButton}
              onClick={() => panel.expand()}
              title="Expand file tree panel"
            >
              ⊞
            </button>
          ) : (
            <button
              className={toolbarButton}
              onClick={() => panel.collapse()}
              title="Collapse file tree panel"
            >
              ⊠
            </button>
          )}
          <button
            className={toolbarButton}
            onClick={() => refreshStatus()}
            title="Refresh git status"
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
        <FileContentViewer
          sessionId={sessionId}
          filePath={selectedPath}
          baseUrl={baseUrl}
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
