"use client";

import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { FileStatus, FileChange } from "@/gen/session/v1/types_pb";
import { useVcsStatus } from "@/lib/hooks/useVcsStatus";
import { FileTree } from "./FileTree";
import { FileContentViewer } from "./FileContentViewer";
import { useSessionVcsContext } from "@/lib/contexts/SessionVcsContext";
import {
  container, treePane, contentPane, toolbar, searchInput,
  toolbarLabel, toolbarButton, searchCount, treeWrapper,
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
  const searchInputRef = useRef<HTMLInputElement>(null);
  const fileTreeCollapseRef = useRef<(() => void) | null>(null);

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
    },
    [onSelectedPathChange]
  );

  // Apply initialSelectedPath changes from parent (VCS cross-link).
  useEffect(() => {
    if (initialSelectedPath !== undefined && initialSelectedPath !== selectedPath) {
      setSelectedPath(initialSelectedPath);
    }
  }, [initialSelectedPath]); // eslint-disable-line react-hooks/exhaustive-deps

  // Cmd+F / Ctrl+F focuses the search input.
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
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  return (
    <div className={container}>
      {/* Left pane: file tree */}
      <div className={treePane}>
        <div className={toolbar}>
          <input
            ref={searchInputRef}
            type="search"
            className={searchInput}
            placeholder="Search files… (⌘F)"
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
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
            onClick={() => fileTreeCollapseRef.current?.()}
            title="Collapse all directories"
          >
            ⊟
          </button>
          <button
            className={toolbarButton}
            onClick={() => refreshStatus()}
            title="Refresh git status"
            disabled={vcsLoading}
          >
            {vcsLoading ? "⟳" : "↺"}
          </button>
        </div>
        <div className={treeWrapper}>
          <FileTree
            sessionId={sessionId}
            baseUrl={baseUrl}
            onFileSelect={handleFileSelect}
            gitStatusMap={gitStatusMap}
            selectedPath={selectedPath}
            includeIgnored={includeIgnored}
            searchTerm={searchTerm}
            onCollapseAllRef={(fn) => { fileTreeCollapseRef.current = fn; }}
            onSearchResults={(count, truncated) => {
              setSearchResultCount(count);
              setSearchResultTruncated(truncated);
            }}
          />
        </div>
      </div>

      {/* Right pane: file content */}
      <div className={contentPane}>
        <FileContentViewer
          sessionId={sessionId}
          filePath={selectedPath}
          baseUrl={baseUrl}
        />
      </div>
    </div>
  );
}
