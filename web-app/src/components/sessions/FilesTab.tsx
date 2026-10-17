"use client";

import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { useFilesFlags } from "@/lib/hooks/useFilesFlags";
import { useFilesV2, type FilesV2 } from "@/lib/hooks/useFilesV2";
import type { OpenSource } from "@/lib/hooks/useOpenBuffers";
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
import { FilesV2Chrome } from "./files/FilesV2Chrome";
import { FilesV2Footer } from "./files/FilesV2Footer";
import { useActiveBufferWait } from "./files/ActiveBufferWait";
import { SearchPanel, SearchPaneTabs, type SearchPanelHandle } from "./files/SearchPanel";
import type { QuickfixView } from "./files/SearchResults";
import { useSearchPane } from "@/lib/hooks/useSearchPane";
import { tabPanel } from "./files/FilesV2Chrome.css";
import { FilesTreeToolbar } from "./FilesTreeToolbar";
import {
  container, treePane, treePaneCollapsed, contentPane, treeWrapper, leftView,
  mobilePaneHidden, mobilePaneVisible, mobileBackButton, mobileSearchButton,
} from "./FilesTab.css";

// ---- Props ----

interface FilesTabProps {
  sessionId: string;
  baseUrl: string;
  /** Path to pre-select when the tab opens (e.g. from VCS panel cross-link). */
  initialSelectedPath?: string | null;
  onSelectedPathChange?: (path: string | null) => void;
  /** Whether the session's agent is running, for the "this may be the agent" stale-banner wording. */
  sessionRunning?: boolean;
  /** Shown in the Search panel's empty state. */
  sessionName?: string;
  /** Resume action for a paused session (Search refuses without a worktree). */
  onResumeSession?: (trigger: HTMLElement) => void;
}

// ---- Component ----

const RECENT_LIMIT = 8;
const PANEL_ID = "files-editor-panel";

/** Shared layout; where the selection and recent list come from is the caller's business. */
interface FilesTabViewProps extends FilesTabProps {
  selectedPath: string | null;
  recentPaths: string[];
  onOpen: (path: string, source?: OpenSource) => void;
  /** Present only with files:viewer-v2: tabs, jumplist, outline and stale detection. */
  v2?: FilesV2;
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
function BufferedFilesTab({ sessionId, initialSelectedPath, onSelectedPathChange, sessionRunning = false, ...rest }: FilesTabProps) {
  const v2 = useFilesV2(sessionId, sessionRunning, { initialSelectedPath, onSelectedPathChange });
  const { open } = v2.bridge;
  const onOpen = useCallback((path: string, source?: OpenSource) => open(path, { source }), [open]);
  return (
    <FilesTabView
      {...rest}
      sessionId={sessionId}
      initialSelectedPath={initialSelectedPath}
      selectedPath={v2.buffers.selectedPath}
      recentPaths={v2.recentPaths}
      onOpen={onOpen}
      v2={v2}
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
  v2,
  sessionName,
  onResumeSession,
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
    (path: string, source?: OpenSource) => {
      onOpen(path, source);
      setMobilePane("content");
    },
    [onOpen]
  );

  const jumpTo = v2?.bridge.jumpTo;
  const onDeepLink = useMemo(() => jumpTo && ((path: string, line = 1) => jumpTo(path, line, { pinned: true, source: "deeplink" })), [jumpTo]);
  useFilesDeepLink({ initialSelectedPath, tree: fileTreeRef, selectedPath, onJump: onDeepLink });
  const openQuickOpen = useCallback(() => setIsQuickOpenOpen(true), []);
  const searchRef = useRef<SearchPanelHandle>(null);
  const [quickfix, setQuickfix] = useState<QuickfixView | null>(null);
  const search = useSearchPane(v2, {
    expandPanel: panel.expand,
    showTreePane: useCallback(() => setMobilePane("tree"), []),
    showContentPane: useCallback(() => setMobilePane("content"), []),
  });
  useFilesShortcuts({ searchInputRef, collapsed: panel.collapsed, onQuickOpen: openQuickOpen, onSearch: v2 ? search.openSearch : undefined,
    onNextHit: v2 ? () => searchRef.current?.step(1) : undefined, onPrevHit: v2 ? () => searchRef.current?.step(-1) : undefined, ...v2?.shortcuts });
  const wait = useActiveBufferWait(v2, fileTreeRef);

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
        data-testid="files-tree-pane"
        style={{ width: panel.collapsed ? 0 : panel.width }}
      >
        {v2 && <SearchPaneTabs pane={search.leftPane} onPane={search.chooseLeftPane} />}
        <div className={leftView} hidden={search.leftPane === "search"}>
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
            onSelect={(path) => handleFileSelect(path, "recent")}
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
        {v2 && (
          <div className={leftView} hidden={search.leftPane !== "search"}>
            <SearchPanel
              ref={searchRef}
              onQuickfix={setQuickfix}
              sessionId={sessionId}
              sessionName={sessionName}
              visible={search.leftPane === "search"}
              focusToken={search.focusToken}
              dirtyPaths={search.dirtyPaths}
              openHit={search.openHit}
              onEscape={v2.bridge.focusEditor}
              onResumeSession={onResumeSession ? () => onResumeSession(document.activeElement as HTMLElement) : undefined}
            />
          </div>
        )}
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
        {v2 && <FilesV2Chrome v2={v2} fileTreeRef={fileTreeRef} panelId={PANEL_ID} />}
        <div className={tabPanel} role={v2 ? "tabpanel" : undefined} id={PANEL_ID} aria-labelledby={v2 ? `files-tab-${v2.buffers.state.activeIndex}` : undefined}>
          {wait.node}
          <div hidden={wait.hideViewer} style={wait.hideViewer ? undefined : { display: "contents" }}>
            <FileContentViewer
              sessionId={sessionId}
              filePath={selectedPath}
              baseUrl={baseUrl}
              diffContent={diff?.content}
              reloadKey={v2?.bridge.reloadKey}
              onEditorReady={v2?.bridge.onEditorReady}
            />
          </div>
        </div>
        {v2 && <FilesV2Footer v2={v2} quickfix={quickfix} />}
      </div>

      {isQuickOpenOpen && (
        <QuickOpenPalette
          sessionId={sessionId}
          baseUrl={baseUrl}
          recentPaths={recentPaths}
          commands={v2?.paletteCommands}
          onSelect={(path) => {
            setIsQuickOpenOpen(false);
            handleFileSelect(path, "palette");
            fileTreeRef.current?.revealPath(path);
          }}
          onClose={() => setIsQuickOpenOpen(false)}
        />
      )}
    </div>
  );
}
