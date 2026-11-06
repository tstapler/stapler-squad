"use client";

import { useMemo, useState, type RefObject } from "react";
import type { FilesV2 } from "@/lib/hooks/useFilesV2";
import type { OpenSource } from "@/lib/hooks/useOpenBuffers";
import type { useSearchPane } from "@/lib/hooks/useSearchPane";
import type { useResizablePanel } from "@/lib/hooks/useResizablePanel";
import { useSessionVcsContext } from "@/lib/contexts/SessionVcsContext";
import { buildGitStatusMap, buildLineStatsMap } from "@/lib/utils/gitStatus";
import { FileTree } from "../FileTree";
import type { FileTreeHandle, SortMode } from "../FileTree";
import { FilesTreeToolbar } from "../FilesTreeToolbar";
import { RecentFilesSection } from "../RecentFilesSection";
import { SearchPanel, SearchPaneTabs, type SearchPanelHandle } from "./SearchPanel";
import type { QuickfixView } from "./SearchResults";
import { leftView, treeWrapper } from "../FilesTab.css";

interface FilesLeftPaneProps {
  sessionId: string;
  baseUrl: string;
  v2?: FilesV2;
  selectedPath: string | null;
  recentPaths: string[];
  onSelect: (path: string, source?: OpenSource) => void;
  searchInputRef: RefObject<HTMLInputElement | null>;
  fileTreeRef: RefObject<FileTreeHandle | null>;
  searchRef: RefObject<SearchPanelHandle | null>;
  panel: Pick<ReturnType<typeof useResizablePanel>, "collapsed" | "collapse" | "expand">;
  search: ReturnType<typeof useSearchPane>;
  onQuickfix: (view: QuickfixView | null) => void;
  sessionName?: string;
  onResumeSession?: (trigger: HTMLElement) => void;
  /** Mobile layout: Files and Search are panes of the bottom bar, not an Explorer | Search header. */
  mobile: boolean;
}

/** The Explorer (toolbar, recent files, tree) and, with files:viewer-v2, the Search panel; both stay mounted. */
export function FilesLeftPane(p: FilesLeftPaneProps) {
  const { v2, search } = p;
  const [includeIgnored, setIncludeIgnored] = useState(false);
  const [sortBy, setSortBy] = useState<SortMode>("name");
  const [filterChangedOnly, setFilterChangedOnly] = useState(false);
  const [searchTerm, setSearchTerm] = useState("");
  const [resultCount, setResultCount] = useState<number | null>(null);
  const [resultTruncated, setResultTruncated] = useState(false);

  // VCS status comes from shared context, with no independent fetch.
  const { status, statusLoading, refreshStatus } = useSessionVcsContext();
  const changedFiles = useMemo(() => {
    if (!status) return [];
    return [...status.stagedFiles, ...status.unstagedFiles, ...status.untrackedFiles];
  }, [status]);
  const gitStatusMap = useMemo(() => buildGitStatusMap(changedFiles), [changedFiles]);
  const lineStatsMap = useMemo(() => buildLineStatsMap(changedFiles), [changedFiles]);

  return (
    <>
      {v2 && !p.mobile && <SearchPaneTabs pane={search.leftPane} onPane={search.chooseLeftPane} />}
      <div className={leftView} hidden={search.leftPane === "search"}>
        <FilesTreeToolbar
          searchInputRef={p.searchInputRef}
          fileTreeRef={p.fileTreeRef}
          searchTerm={searchTerm}
          onSearchTerm={setSearchTerm}
          searchResultCount={resultCount}
          searchResultTruncated={resultTruncated}
          includeIgnored={includeIgnored}
          onIncludeIgnored={setIncludeIgnored}
          filterChangedOnly={filterChangedOnly}
          onFilterChangedOnly={setFilterChangedOnly}
          sortBy={sortBy}
          onToggleSort={() => setSortBy((prev) => (prev === "name" ? "type" : "name"))}
          collapsed={p.panel.collapsed}
          onCollapse={p.panel.collapse}
          onExpand={p.panel.expand}
          vcsLoading={statusLoading}
          onRefreshStatus={refreshStatus}
        />
        <RecentFilesSection paths={p.recentPaths} selectedPath={p.selectedPath} onSelect={(path) => p.onSelect(path, "recent")} />
        <div className={treeWrapper}>
          <FileTree
            ref={p.fileTreeRef}
            sessionId={p.sessionId}
            baseUrl={p.baseUrl}
            onFileSelect={p.onSelect}
            gitStatusMap={gitStatusMap}
            lineStatsMap={lineStatsMap}
            sortBy={sortBy}
            filterChangedOnly={filterChangedOnly}
            selectedPath={p.selectedPath}
            includeIgnored={includeIgnored}
            searchTerm={searchTerm}
            onSearchResults={(count, truncated) => {
              setResultCount(count);
              setResultTruncated(truncated);
            }}
          />
        </div>
      </div>
      {v2 && (
        <div className={leftView} hidden={search.leftPane !== "search"}>
          <SearchPanel
            ref={p.searchRef}
            onQuickfix={p.onQuickfix}
            sessionId={p.sessionId}
            sessionName={p.sessionName}
            visible={search.leftPane === "search"}
            focusToken={search.focusToken}
            dirtyPaths={search.dirtyPaths}
            openHit={search.openHit}
            onEscape={v2.bridge.focusEditor}
            onResumeSession={p.onResumeSession ? () => p.onResumeSession?.(document.activeElement as HTMLElement) : undefined}
          />
        </div>
      )}
    </>
  );
}
