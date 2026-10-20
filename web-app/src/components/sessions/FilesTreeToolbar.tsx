"use client";

import type { RefObject } from "react";
import type { FileTreeHandle, SortMode } from "./FileTree";
import {
  toolbar, searchInput, toolbarLabel, toolbarButton, searchCount, toolbarButtonMobileHidden, toolbarDivider,
} from "./FilesTab.css";

export interface FilesTreeToolbarProps {
  searchInputRef: RefObject<HTMLInputElement | null>;
  fileTreeRef: RefObject<FileTreeHandle | null>;
  searchTerm: string;
  onSearchTerm: (term: string) => void;
  searchResultCount: number | null;
  searchResultTruncated: boolean;
  includeIgnored: boolean;
  onIncludeIgnored: (on: boolean) => void;
  filterChangedOnly: boolean;
  onFilterChangedOnly: (on: boolean) => void;
  sortBy: SortMode;
  onToggleSort: () => void;
  collapsed: boolean;
  onCollapse: () => void;
  onExpand: () => void;
  vcsLoading: boolean;
  onRefreshStatus: () => void;
}

const desktopOnly = `${toolbarButton} ${toolbarButtonMobileHidden}`;

/** The Explorer pane's search box and option row. */
export function FilesTreeToolbar(p: FilesTreeToolbarProps) {
  const nextSort = p.sortBy === "name" ? "type" : "name";
  return (
    <div className={toolbar}>
      <input
        ref={p.searchInputRef}
        type="search"
        className={searchInput}
        placeholder="Search files…"
        value={p.searchTerm}
        onChange={(e) => p.onSearchTerm(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            p.onSearchTerm("");
            p.searchInputRef.current?.blur();
          }
        }}
        enterKeyHint="search"
        aria-label="Search files"
      />
      {p.searchResultCount !== null && p.searchTerm.length >= 2 && (
        <span
          className={searchCount}
          title={p.searchResultTruncated ? "Results truncated at 500" : undefined}
          aria-live="polite"
          aria-atomic="true"
        >
          {p.searchResultCount}{p.searchResultTruncated ? "+" : ""} match{p.searchResultCount !== 1 ? "es" : ""}
        </span>
      )}
      <label className={toolbarLabel} title="Show gitignored files">
        <input type="checkbox" checked={p.includeIgnored} onChange={(e) => p.onIncludeIgnored(e.target.checked)} />
        Ignored
      </label>
      <label className={toolbarLabel} title="Show only files with git changes">
        <input type="checkbox" checked={p.filterChangedOnly} onChange={(e) => p.onFilterChangedOnly(e.target.checked)} />
        Changed
      </label>
      <button className={desktopOnly} onClick={p.onToggleSort} title={`Sort by ${nextSort}`} aria-label={`Sort by ${nextSort}`}>
        Sort: {p.sortBy === "name" ? "Name" : "Type"}
      </button>
      <button className={desktopOnly} onClick={() => p.fileTreeRef.current?.collapseAll()} title="Collapse all directories" aria-label="Collapse all directories">
        ⊟
      </button>
      <div className={toolbarDivider} />
      <button
        className={desktopOnly}
        onClick={p.collapsed ? p.onExpand : p.onCollapse}
        title={p.collapsed ? "Expand file tree panel" : "Collapse file tree panel"}
        aria-label={p.collapsed ? "Expand file tree panel" : "Collapse file tree panel"}
      >
        {p.collapsed ? "⊞" : "⊠"}
      </button>
      <button className={toolbarButton} onClick={p.onRefreshStatus} title="Refresh git status" aria-label="Refresh git status" disabled={p.vcsLoading}>
        {p.vcsLoading ? "⟳" : "↺"}
      </button>
    </div>
  );
}
