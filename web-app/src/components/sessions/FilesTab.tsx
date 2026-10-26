"use client";

import { useState, useEffect, useCallback, useMemo, useRef } from "react";
import { useFilesFlags } from "@/lib/hooks/useFilesFlags";
import { useFilesV2, type FilesV2 } from "@/lib/hooks/useFilesV2";
import type { OpenSource } from "@/lib/hooks/useOpenBuffers";
import { useFilesShortcuts } from "@/lib/hooks/useFilesShortcuts";
import { useFilesDeepLink } from "@/lib/hooks/useFilesDeepLink";
import { FileTransportProvider } from "@/lib/hooks/useFileTransport";
import type { FileTreeHandle } from "./FileTree";
import { FileContentViewer } from "./FileContentViewer";
import { useSessionVcsContext } from "@/lib/contexts/SessionVcsContext";
import { useResizablePanel } from "@/lib/hooks/useResizablePanel";
import { useFilesLayout } from "@/lib/hooks/useFilesLayout";
import { useMobilePane } from "@/lib/hooks/useMobilePane";
import { TreeResizeHandle } from "./TreeResizeHandle";
import { QuickOpenPalette } from "./QuickOpenPalette";
import { FilesV2Chrome } from "./files/FilesV2Chrome";
import { FilesV2Footer } from "./files/FilesV2Footer";
import { FilesLeftPane } from "./files/FilesLeftPane";
import { MobileFilesLayout } from "./files/MobileFilesLayout";
import { useActiveBufferWait } from "./files/ActiveBufferWait";
import type { SearchPanelHandle } from "./files/SearchPanel";
import type { QuickfixView } from "./files/SearchResults";
import { useSearchPane } from "@/lib/hooks/useSearchPane";
import { AccessibleSkipLink } from "./files/AccessibleView";
import { tabPanel } from "./files/FilesV2Chrome.css";
import { container, treePane, treePaneCollapsed, contentPane, mobileBackButton, mobileSearchButton, expandStrip } from "./FilesTab.css";

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
  /** The Files tab is the visible tab; false pauses on-disk freshness polling (StatFile). Defaults to true. */
  tabActive?: boolean;
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
function BufferedFilesTab({ sessionId, initialSelectedPath, onSelectedPathChange, sessionRunning = false, tabActive = true, ...rest }: FilesTabProps) {
  const v2 = useFilesV2(sessionId, sessionRunning, { initialSelectedPath, onSelectedPathChange }, tabActive);
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
  const [isQuickOpenOpen, setIsQuickOpenOpen] = useState(false);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const fileTreeRef = useRef<FileTreeHandle>(null);
  const searchRef = useRef<SearchPanelHandle>(null);
  const [quickfix, setQuickfix] = useState<QuickfixView | null>(null);

  const panel = useResizablePanel({ storageKey: "filestab.treeWidth", defaultWidth: 260, minWidth: 160, maxWidthFraction: 0.5 });
  const { layout, containerWidth } = useFilesLayout(panel.containerRef);
  const mobile = layout === "mobile";
  const nav = useMobilePane(mobile);
  const { go } = nav;
  const { diff } = useSessionVcsContext();

  const handleFileSelect = useCallback(
    (path: string, source?: OpenSource) => {
      onOpen(path, source);
      go("editor", "tap_hit");
    },
    [onOpen, go]
  );

  const jumpTo = v2?.bridge.jumpTo;
  const onDeepLink = useMemo(
    () => jumpTo && ((path: string, line = 1) => { jumpTo(path, line, { pinned: true, source: "deeplink" }); go("editor", "tap_hit"); }),
    [jumpTo, go],
  );
  useFilesDeepLink({ initialSelectedPath, tree: fileTreeRef, selectedPath, onJump: onDeepLink });
  const openQuickOpen = useCallback(() => setIsQuickOpenOpen(true), []);
  const search = useSearchPane(v2, {
    expandPanel: panel.expand,
    showTreePane: useCallback(() => go("search", "bottom_bar"), [go]),
    showContentPane: useCallback(() => go("editor", "tap_hit"), [go]),
  });
  // On mobile the Files and Search panes pick which left view shows; the editor pane leaves it as it was.
  const { showLeftPane } = search;
  useEffect(() => {
    if (mobile && nav.pane !== "editor") showLeftPane(nav.pane === "search" ? "search" : "explorer");
  }, [mobile, nav.pane, showLeftPane]);
  useFilesShortcuts({ searchInputRef, collapsed: panel.collapsed, onQuickOpen: openQuickOpen, onSearch: v2 ? search.openSearch : undefined,
    onNextHit: v2 ? () => searchRef.current?.step(1) : undefined, onPrevHit: v2 ? () => searchRef.current?.step(-1) : undefined, ...v2?.shortcuts });
  const wait = useActiveBufferWait(v2, fileTreeRef);
  const active = v2?.buffers.active;

  const treePaneClasses = [treePane, panel.collapsed && !mobile ? treePaneCollapsed : ""].filter(Boolean).join(" ");

  return (
    <div className={container} ref={panel.containerRef} data-testid="files-tab" data-layout={layout} data-pane={nav.pane} data-chrome={v2 ? "v2" : "legacy"}>
      <MobileFilesLayout
        active={mobile}
        v2={v2}
        pane={nav.pane}
        origin={nav.origin}
        containerWidth={containerWidth}
        onPane={nav.go}
        onBack={nav.back}
        onQuickOpen={openQuickOpen}
        onSearch={search.openSearch}
        quickfix={quickfix}
      >
        {/* Left pane: file tree and search */}
        <div className={treePaneClasses} data-testid="files-tree-pane" style={{ width: panel.collapsed ? 0 : panel.width }}>
          <FilesLeftPane
            sessionId={sessionId}
            baseUrl={baseUrl}
            v2={v2}
            selectedPath={selectedPath}
            recentPaths={recentPaths}
            onSelect={handleFileSelect}
            searchInputRef={searchInputRef}
            fileTreeRef={fileTreeRef}
            searchRef={searchRef}
            panel={panel}
            search={search}
            onQuickfix={setQuickfix}
            sessionName={sessionName}
            onResumeSession={onResumeSession}
            mobile={mobile}
          />
        </div>

        {!mobile && !panel.collapsed && <TreeResizeHandle {...panel.handleProps} />}
        {!mobile && panel.collapsed && (
          <button className={expandStrip} onClick={panel.expand} title="Expand file tree panel" aria-label="Expand file tree panel">
            ⊞
          </button>
        )}

        {/* Right pane: file content */}
        <div className={contentPane}>
          <button className={mobileBackButton} onClick={() => nav.back()}>
            ← Files
          </button>
          <button className={mobileSearchButton} onClick={openQuickOpen} aria-label="Search files">
            🔍
          </button>
          {v2 && <FilesV2Chrome v2={v2} fileTreeRef={fileTreeRef} panelId={PANEL_ID} mobile={mobile} onQuickOpen={openQuickOpen} onSearch={search.openSearch} />}
          <div className={tabPanel} role={v2 ? "tabpanel" : undefined} id={PANEL_ID} aria-labelledby={v2 ? `files-tab-${v2.buffers.state.activeIndex}` : undefined}>
            {v2 && active && <AccessibleSkipLink on={v2.accessible.on} onToggle={() => v2.accessible.toggle("skip_link")} />}
            {wait.node}
            <div hidden={wait.hideViewer} style={wait.hideViewer ? undefined : { display: "contents" }}>
              <FileContentViewer
                sessionId={sessionId}
                filePath={selectedPath}
                baseUrl={baseUrl}
                diffContent={diff?.content}
                reloadKey={v2?.bridge.reloadKey}
                onEditorReady={v2?.bridge.onEditorReady}
                accessible={v2?.accessible.on}
                dirty={v2?.buffers.active?.status === "loaded" && v2.buffers.active.dirty}
                source={v2?.viewerSource}
              />
            </div>
          </div>
          {v2 && <FilesV2Footer v2={v2} quickfix={mobile ? null : quickfix} />}
        </div>
      </MobileFilesLayout>

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
