"use client";

import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { useFilesFlags } from "@/lib/hooks/useFilesFlags";
import { useOpenBuffers } from "@/lib/hooks/useOpenBuffers";
import { useFilesShortcuts } from "@/lib/hooks/useFilesShortcuts";
import { useFilesDeepLink } from "@/lib/hooks/useFilesDeepLink";
import { FileTransportProvider } from "@/lib/hooks/useFileTransport";
import { FileTree } from "./FileTree";
import type { FileTreeHandle, SortMode } from "./FileTree";
import { FileContentViewer } from "./FileContentViewer";
import { useSessionVcsContext } from "@/lib/contexts/SessionVcsContext";
import { useResizablePanel } from "@/lib/hooks/useResizablePanel";
import { buildGitStatusMap, buildLineStatsMap } from "@/lib/utils/gitStatus";
import { TreeResizeHandle } from "./TreeResizeHandle";
import { RecentFilesSection } from "./RecentFilesSection";
import { QuickOpenPalette } from "./QuickOpenPalette";
import { FilesTreeToolbar } from "./FilesTreeToolbar";
import {
  container, treePane, treePaneCollapsed, contentPane, treeWrapper,
  mobilePaneHidden, mobilePaneVisible, mobileBackButton, mobileSearchButton,
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

const RECENT_LIMIT = 8;

/** Shared layout; where the selection and recent list come from is the caller's business. */
interface FilesTabViewProps extends FilesTabProps {
  selectedPath: string | null;
  recentPaths: string[];
  onOpen: (path: string) => void;
}

export function FilesTab(props: FilesTabProps) {
  return (
    <FileTransportProvider>
      <FilesTabGate {...props} />
    </FileTransportProvider>
  );
}

function FilesTabGate(props: FilesTabProps) {
  const { status, viewerV2 } = useFilesFlags();
  if (status === "loading") return <div className={container} data-testid="files-tab-skeleton" aria-busy="true" />;
  return viewerV2 ? <BufferedFilesTab {...props} /> : <LegacyFilesTab {...props} />;
}

/** files:viewer-v2 on: selection is the active buffer of the open-buffers reducer. */
function BufferedFilesTab({ sessionId, initialSelectedPath, onSelectedPathChange, ...rest }: FilesTabProps) {
  const buffers = useOpenBuffers(sessionId, { initialSelectedPath, onSelectedPathChange });
  const recentPaths = useMemo(() => buffers.mru.slice(0, RECENT_LIMIT), [buffers.mru]);
  const { open } = buffers;
  const onOpen = useCallback((path: string) => open(path), [open]);
  return (
    <FilesTabView
      {...rest}
      sessionId={sessionId}
      initialSelectedPath={initialSelectedPath}
      selectedPath={buffers.selectedPath}
      recentPaths={recentPaths}
      onOpen={onOpen}
    />
  );
}

/** files:viewer-v2 off: the pre-revamp single-selection behavior. */
function LegacyFilesTab({ initialSelectedPath, onSelectedPathChange, ...rest }: FilesTabProps) {
  const [selectedPath, setSelectedPath] = useState<string | null>(initialSelectedPath ?? null);
  const [recentPaths, setRecentPaths] = useState<string[]>([]);
  const onOpen = useCallback(
    (path: string) => {
      setSelectedPath(path);
      onSelectedPathChange?.(path);
      setRecentPaths((prev) => [path, ...prev.filter((p) => p !== path)].slice(0, RECENT_LIMIT));
    },
    [onSelectedPathChange]
  );
  useEffect(() => {
    if (initialSelectedPath !== undefined) setSelectedPath(initialSelectedPath);
  }, [initialSelectedPath]);
  return (
    <FilesTabView {...rest} initialSelectedPath={initialSelectedPath} selectedPath={selectedPath} recentPaths={recentPaths} onOpen={onOpen} />
  );
}

function FilesTabView({
  sessionId,
  baseUrl,
  initialSelectedPath,
  selectedPath,
  recentPaths,
  onOpen,
}: FilesTabViewProps) {
  const [includeIgnored, setIncludeIgnored] = useState(false);
  const [sortBy, setSortBy] = useState<SortMode>("name");
  const [filterChangedOnly, setFilterChangedOnly] = useState(false);
  const [searchTerm, setSearchTerm] = useState("");
  const [searchResultCount, setSearchResultCount] = useState<number | null>(null);
  const [searchResultTruncated, setSearchResultTruncated] = useState(false);
  const [mobilePane, setMobilePane] = useState<"tree" | "content">("tree");
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

  const handleFileSelect = useCallback(
    (path: string) => {
      onOpen(path);
      setMobilePane("content");
    },
    [onOpen]
  );

  useFilesDeepLink(initialSelectedPath, fileTreeRef);
  const openQuickOpen = useCallback(() => setIsQuickOpenOpen(true), []);
  useFilesShortcuts({ searchInputRef, collapsed: panel.collapsed, onQuickOpen: openQuickOpen });

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
        <FilesTreeToolbar
          searchInputRef={searchInputRef}
          fileTreeRef={fileTreeRef}
          searchTerm={searchTerm}
          onSearchTerm={setSearchTerm}
          searchResultCount={searchResultCount}
          searchResultTruncated={searchResultTruncated}
          includeIgnored={includeIgnored}
          onIncludeIgnored={setIncludeIgnored}
          filterChangedOnly={filterChangedOnly}
          onFilterChangedOnly={setFilterChangedOnly}
          sortBy={sortBy}
          onToggleSort={() => setSortBy((prev) => (prev === "name" ? "type" : "name"))}
          collapsed={panel.collapsed}
          onCollapse={panel.collapse}
          onExpand={panel.expand}
          vcsLoading={vcsLoading}
          onRefreshStatus={refreshStatus}
        />
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
