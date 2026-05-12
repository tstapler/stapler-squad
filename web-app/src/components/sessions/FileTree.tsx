"use client";

import { useState, useCallback, useEffect, useRef, useMemo } from "react";
import { Tree } from "react-arborist";
import type { NodeApi, TreeApi } from "react-arborist";
import type { FileNode } from "@/gen/session/v1/types_pb";
import { fetchDirectoryFiles, searchFiles } from "@/lib/hooks/useFileService";
import {
  container, loading as loadingClass, error as errorClass, retryButton, empty,
  node as nodeClass, selected, keyboardFocused, nodeInner, icon as iconClass, name as nameClass, ignored,
  symlinkBadge, statusBadge, spinner, inlineError,
  searchContainer, searchInput, toolbar, toolbarButton, toolbarLabel,
  treeWrapper, mark, searchEmpty, searchTruncated as searchTruncatedClass,
} from "./FileTree.css";

// ---- Data model ----

export interface TreeNode {
  id: string;        // full relative path (unique within worktree)
  name: string;
  isDir: boolean;
  size: bigint;
  gitStatus: string;
  isSymlink: boolean;
  symlinkTarget: string;
  isIgnored: boolean;
  children?: TreeNode[]; // undefined = not loaded, [] = empty dir
}

// Git status colors.
const GIT_STATUS_COLORS: Record<string, string> = {
  M: "#cca700",
  A: "#2ea043",
  D: "#f85149",
  "?": "#3fb950",
  R: "#58a6ff",
  U: "#f85149",
};

// ---- Props ----

interface FileTreeProps {
  sessionId: string;
  baseUrl: string;
  /** Called when a file (non-directory) is selected. */
  onFileSelect: (path: string) => void;
  /** Map of relative path → git status letter. */
  gitStatusMap?: Map<string, string>;
  /** Selected file path (for visual highlight). */
  selectedPath?: string | null;
  /** Whether to include gitignored files. */
  includeIgnored?: boolean;
  /** Search/filter term — filters tree by name/path substring. */
  searchTerm?: string;
  /** Called with a collapseAll function so parents can trigger collapse. */
  onCollapseAllRef?: (fn: () => void) => void;
  /** Called when search results change (count, truncated). null = browse mode. */
  onSearchResults?: (count: number | null, truncated: boolean) => void;
}

// ---- Helpers ----

function fileNodeToTreeNode(fn: FileNode): TreeNode {
  return {
    id: fn.path || fn.name,
    name: fn.name,
    isDir: fn.isDir,
    size: fn.size,
    gitStatus: fn.gitStatus,
    isSymlink: fn.isSymlink,
    symlinkTarget: fn.symlinkTarget,
    isIgnored: fn.isIgnored,
    children: fn.isDir ? undefined : undefined,
  };
}

/**
 * Build tree data from the directory contents map.
 * Recursively attaches loaded children to each directory node.
 * Exported for unit testing.
 */
export function buildTreeData(
  nodes: TreeNode[],
  dirContents: Map<string, TreeNode[]>
): TreeNode[] {
  return nodes.map((node) => {
    if (!node.isDir) return node;
    const loaded = dirContents.get(node.id);
    if (loaded === undefined) {
      // children: [] makes isLeaf=false (react-arborist checks Array.isArray).
      // This allows node.toggle() to fire, which triggers onToggle → handleToggle → loadDirectory.
      // Children will be populated lazily after the first toggle.
      return { ...node, children: [] };
    }
    return {
      ...node,
      children: buildTreeData(loaded, dirContents),
    };
  });
}

/**
 * Compute which directories have any git-modified descendants.
 */
function computeDirStatuses(
  nodes: TreeNode[],
  gitStatusMap: Map<string, string>,
  result: Map<string, string>
): boolean {
  let anyStatus = false;
  for (const node of nodes) {
    if (!node.isDir) {
      const status = gitStatusMap.get(node.id);
      if (status) {
        anyStatus = true;
      }
    } else if (node.children) {
      const childHas = computeDirStatuses(node.children, gitStatusMap, result);
      if (childHas) {
        result.set(node.id, "●");
        anyStatus = true;
      }
    }
  }
  return anyStatus;
}

/**
 * Build a nested TreeNode tree from a flat list of FileNode search results.
 * Ancestor directories are synthesised from file path segments.
 */
function buildSearchTree(files: FileNode[]): TreeNode[] {
  const nodeMap = new Map<string, TreeNode>();

  for (const file of files) {
    const filePath = file.path || file.name;
    const parts = filePath.split("/");

    // Create ancestor directory nodes for each path segment.
    for (let i = 1; i < parts.length; i++) {
      const dirPath = parts.slice(0, i).join("/");
      if (!nodeMap.has(dirPath)) {
        nodeMap.set(dirPath, {
          id: dirPath,
          name: parts[i - 1],
          isDir: true,
          size: BigInt(0),
          gitStatus: "",
          isSymlink: false,
          symlinkTarget: "",
          isIgnored: false,
          children: [],
        });
      }
    }

    // Create the file node (reuse existing converter for consistency).
    nodeMap.set(filePath, fileNodeToTreeNode(file));
  }

  // Wire children into parents.
  const roots: TreeNode[] = [];
  for (const [path, node] of nodeMap) {
    const lastSlash = path.lastIndexOf("/");
    if (lastSlash === -1) {
      roots.push(node);
    } else {
      const parentPath = path.slice(0, lastSlash);
      const parent = nodeMap.get(parentPath);
      if (parent?.children) {
        parent.children.push(node);
      }
    }
  }

  // Sort recursively: directories first, then alphabetical.
  function sortNodes(nodes: TreeNode[]): TreeNode[] {
    nodes.sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
      return a.name.toLowerCase().localeCompare(b.name.toLowerCase());
    });
    for (const node of nodes) {
      if (node.children) sortNodes(node.children);
    }
    return nodes;
  }

  return sortNodes(roots);
}

// ---- Node renderer ----

interface NodeRendererProps {
  node: NodeApi<TreeNode>;
  style: React.CSSProperties;
  dragHandle?: (el: HTMLDivElement | null) => void;
  gitStatusMap: Map<string, string>;
  dirStatusMap: Map<string, string>;
  loadingPaths: Set<string>;
  errorPaths: Map<string, string>;
  selectedPath: string | null | undefined;
  includeIgnored: boolean;
  searchTerm: string;
}

function highlightMatch(name: string, term: string): React.ReactNode {
  if (!term) return name;
  const idx = name.toLowerCase().indexOf(term.toLowerCase());
  if (idx === -1) return name;
  return (
    <>
      {name.slice(0, idx)}
      <mark className={mark}>{name.slice(idx, idx + term.length)}</mark>
      {name.slice(idx + term.length)}
    </>
  );
}

function NodeRenderer({
  node,
  style,
  gitStatusMap,
  dirStatusMap,
  loadingPaths,
  errorPaths,
  selectedPath,
  searchTerm,
}: NodeRendererProps) {
  const data = node.data;
  const isSelected = selectedPath === data.id;
  const isLoading = loadingPaths.has(data.id);
  const loadError = errorPaths.get(data.id);

  // Determine git status badge.
  const statusLetter = data.isDir
    ? dirStatusMap.get(data.id)
    : gitStatusMap.get(data.id) || data.gitStatus;
  const statusColor = statusLetter ? GIT_STATUS_COLORS[statusLetter] : undefined;

  const icon = data.isSymlink
    ? "⇢"
    : data.isDir
    ? node.isOpen
      ? "▾"
      : "▸"
    : getFileIcon(data.name);

  return (
    <div
      style={style}
      className={`${nodeClass} ${isSelected ? selected : ""} ${node.isFocused ? keyboardFocused : ""} ${data.isIgnored ? ignored : ""}`}
      onClick={() => {
        // Directories toggle open/close (fires onToggle → handleToggle → loadDirectory).
        // Files/symlinks activate (fires onActivate → onFileSelect).
        if (data.isDir) node.toggle();
        else node.activate();
      }}
    >
      <div
        className={nodeInner}
        style={{ paddingLeft: `${node.level * 16 + 8}px` }}
      >
        <span className={iconClass}>{icon}</span>
        <span className={nameClass}>{highlightMatch(data.name, searchTerm)}</span>
        {data.isSymlink && (
          <span className={symlinkBadge} title={`→ ${data.symlinkTarget}`}>
            symlink
          </span>
        )}
        {isLoading && <span className={spinner} />}
        {loadError && (
          <span className={inlineError} title={loadError}>
            ⚠
          </span>
        )}
        {statusLetter && (
          <span
            className={statusBadge}
            style={{ color: statusColor }}
            title={`Git status: ${statusLetter}`}
          >
            {statusLetter}
          </span>
        )}
      </div>
    </div>
  );
}

function getFileIcon(name: string): string {
  const ext = name.split(".").pop()?.toLowerCase() || "";
  const icons: Record<string, string> = {
    go: "🐹",
    ts: "𝐓",
    tsx: "⚛",
    js: "𝐉",
    jsx: "⚛",
    py: "🐍",
    rs: "🦀",
    md: "📄",
    json: "{}",
    yaml: "⚙",
    yml: "⚙",
    toml: "⚙",
    sh: "💲",
    css: "🎨",
    html: "🌐",
  };
  return icons[ext] || "📄";
}

// ---- Main component ----

// Stable empty map to avoid creating a new Map reference on every render when
// gitStatusMap is not provided, which would break useMemo dependency checks.
const EMPTY_GIT_STATUS_MAP = new Map<string, string>();

export function FileTree({
  sessionId,
  baseUrl,
  onFileSelect,
  gitStatusMap = EMPTY_GIT_STATUS_MAP,
  selectedPath,
  includeIgnored = false,
  searchTerm = "",
  onCollapseAllRef,
  onSearchResults,
}: FileTreeProps) {
  // Map of directory path → loaded TreeNode children.
  const [dirContents, setDirContents] = useState<Map<string, TreeNode[]>>(new Map());
  // Tracks which paths are currently loading.
  const [loadingPaths, setLoadingPaths] = useState<Set<string>>(new Set());
  // Tracks which paths have load errors.
  const [errorPaths, setErrorPaths] = useState<Map<string, string>>(new Map());
  // Root loading/error state.
  const [rootLoading, setRootLoading] = useState(true);
  const [rootError, setRootError] = useState<string | null>(null);

  // Search mode state. null = browse mode, array = search mode.
  const [searchResults, setSearchResults] = useState<TreeNode[] | null>(null);
  const [searchLoading, setSearchLoading] = useState(false);
  const [searchTruncated, setSearchTruncated] = useState(false);

  // Request ID ref prevents stale search responses from overwriting newer results.
  const searchRequestIdRef = useRef(0);
  // Timer ref for debouncing search input.
  const searchTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Track whether we were in search mode to trigger closeAll on exit.
  const wasInSearchModeRef = useRef(false);
  // Snapshot of open node IDs taken just before entering search mode.
  const savedOpenStateRef = useRef<Record<string, boolean>>({});

  const treeRef = useRef<TreeApi<TreeNode> | undefined>(undefined);
  // Tracks the timestamp of the last 'g' keypress for the gg chord.
  const lastGRef = useRef<number>(0);

  // ResizeObserver: track container dimensions for react-window (requires numeric width/height).
  const containerRef = useRef<HTMLDivElement>(null);
  const [dims, setDims] = useState({ w: 300, h: 600 });

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const ro = new ResizeObserver(([entry]) => {
      requestAnimationFrame(() => {
        const { width, height } = entry.contentRect;
        if (width > 0 && height > 0) {
          setDims({ w: Math.floor(width), h: Math.floor(height) });
        }
      });
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  // Register collapseAll callback with parent when treeRef or onCollapseAllRef changes.
  useEffect(() => {
    if (onCollapseAllRef) {
      onCollapseAllRef(() => {
        treeRef.current?.closeAll();
      });
    }
  }, [onCollapseAllRef]);

  // Load a directory's children.
  const loadDirectory = useCallback(
    async (dirPath: string) => {
      if (loadingPaths.has(dirPath)) return;

      setLoadingPaths((prev) => new Set(prev).add(dirPath));
      setErrorPaths((prev) => {
        const next = new Map(prev);
        next.delete(dirPath);
        return next;
      });

      try {
        const response = await fetchDirectoryFiles(
          sessionId,
          dirPath === "." ? "." : dirPath,
          includeIgnored,
          baseUrl
        );
        const nodes = (response.files || []).map(fileNodeToTreeNode);

        setDirContents((prev) => {
          const next = new Map(prev);
          next.set(dirPath, nodes);
          return next;
        });
      } catch (err) {
        const msg = err instanceof Error ? err.message : "Failed to load directory";
        setErrorPaths((prev) => new Map(prev).set(dirPath, msg));
      } finally {
        setLoadingPaths((prev) => {
          const next = new Set(prev);
          next.delete(dirPath);
          return next;
        });
      }
    },
    [sessionId, baseUrl, includeIgnored, loadingPaths]
  );

  // Load root on mount / when session changes.
  useEffect(() => {
    setDirContents(new Map());
    setRootLoading(true);
    setRootError(null);

    fetchDirectoryFiles(sessionId, ".", includeIgnored, baseUrl)
      .then((response) => {
        const nodes = (response.files || []).map(fileNodeToTreeNode);
        setDirContents(new Map([[".", nodes]]));
      })
      .catch((err) => {
        setRootError(err instanceof Error ? err.message : "Failed to load files");
      })
      .finally(() => {
        setRootLoading(false);
      });
  }, [sessionId, baseUrl]); // eslint-disable-line react-hooks/exhaustive-deps

  // Reload when includeIgnored changes.
  useEffect(() => {
    setDirContents(new Map());
    setRootLoading(true);
    setRootError(null);

    fetchDirectoryFiles(sessionId, ".", includeIgnored, baseUrl)
      .then((response) => {
        const nodes = (response.files || []).map(fileNodeToTreeNode);
        setDirContents(new Map([[".", nodes]]));
      })
      .catch((err) => {
        setRootError(err instanceof Error ? err.message : "Failed to load files");
      })
      .finally(() => {
        setRootLoading(false);
      });
  }, [includeIgnored]); // eslint-disable-line react-hooks/exhaustive-deps

  // Debounced backend search: fires when searchTerm changes.
  useEffect(() => {
    if (searchTimerRef.current) {
      clearTimeout(searchTimerRef.current);
    }

    if (!searchTerm || searchTerm.length < 2) {
      // Exit search mode.
      setSearchResults(null);
      setSearchLoading(false);
      setSearchTruncated(false);
      onSearchResults?.(null, false);
      return;
    }

    setSearchLoading(true);

    searchTimerRef.current = setTimeout(async () => {
      const requestId = ++searchRequestIdRef.current;

      try {
        const response = await searchFiles(sessionId, searchTerm, includeIgnored, baseUrl);
        if (requestId !== searchRequestIdRef.current) return; // stale response

        const tree = buildSearchTree(response.files || []);
        setSearchResults(tree);
        setSearchTruncated(response.truncated);
        setSearchLoading(false);
        onSearchResults?.(response.totalMatches, response.truncated);
      } catch {
        if (requestId !== searchRequestIdRef.current) return;
        setSearchResults([]);
        setSearchLoading(false);
        onSearchResults?.(0, false);
      }
    }, 300);

    return () => {
      if (searchTimerRef.current) clearTimeout(searchTimerRef.current);
    };
  }, [searchTerm, sessionId, includeIgnored, baseUrl]); // eslint-disable-line react-hooks/exhaustive-deps

  // Open all tree nodes when entering search mode; restore prior state when leaving.
  useEffect(() => {
    if (searchResults !== null) {
      if (!wasInSearchModeRef.current) {
        // Snapshot expanded state before entering search for restoration on exit.
        savedOpenStateRef.current = { ...(treeRef.current?.openState ?? {}) };
        wasInSearchModeRef.current = true;
      }
      // Delay to allow react-arborist to render the new data before calling openAll.
      const timer = setTimeout(() => {
        treeRef.current?.openAll();
      }, 0);
      return () => clearTimeout(timer);
    } else if (wasInSearchModeRef.current) {
      wasInSearchModeRef.current = false;
      // Restore the browse-mode open state instead of collapsing everything.
      const saved = savedOpenStateRef.current;
      treeRef.current?.closeAll();
      for (const [id, isOpen] of Object.entries(saved)) {
        if (isOpen) treeRef.current?.open(id);
      }
      savedOpenStateRef.current = {};
    }
  }, [searchResults]);

  // Memoized tree computations — only recompute when their inputs actually change.
  const treeData = useMemo(
    () => buildTreeData(dirContents.get(".") ?? [], dirContents),
    [dirContents]
  );
  const displayedData = useMemo(
    () => searchResults ?? treeData,
    [searchResults, treeData]
  );
  const dirStatusMap = useMemo(() => {
    const m = new Map<string, string>();
    computeDirStatuses(displayedData, gitStatusMap, m);
    return m;
  }, [displayedData, gitStatusMap]);

  // Set of all known directory ids across all loaded children.
  // Allows handleToggle to skip non-directory ids without re-traversing the tree.
  const knownDirIds = useMemo(() => {
    const s = new Set<string>();
    for (const nodes of dirContents.values()) {
      for (const node of nodes) {
        if (node.isDir) s.add(node.id);
      }
    }
    return s;
  }, [dirContents]);

  const handleActivate = useCallback(
    (node: NodeApi<TreeNode>) => {
      const data = node.data;
      if (!data.isDir && !data.isSymlink) {
        onFileSelect(data.id);
      }
    },
    [onFileSelect]
  );

  const handleToggle = useCallback(
    (id: string) => {
      // load-bearing guard: prevents openAll() fan-out in search mode.
      if (searchResults !== null) return;
      // Only attempt to load known directory ids; ignore files and unknown ids.
      if (!knownDirIds.has(id)) return;
      // Load if not yet fetched, or retry if a previous load errored.
      if (!dirContents.has(id) || errorPaths.has(id)) {
        loadDirectory(id);
      }
    },
    [dirContents, errorPaths, knownDirIds, loadDirectory, searchResults]
  );

  const handleTreeKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      const tree = treeRef.current;
      if (!tree) return;

      // Don't intercept modified shortcuts (Ctrl+G, Cmd+K, Alt+j, etc.)
      if (e.metaKey || e.ctrlKey || e.altKey) return;

      const focusedNode = tree.focusedNode;
      const visible = tree.visibleNodes;
      if (!visible || visible.length === 0) return;

      switch (e.key) {
        case "j": {
          e.preventDefault();
          const idx = focusedNode ? visible.findIndex((n) => n.id === focusedNode.id) : -1;
          const next = visible[idx + 1];
          if (next) tree.focus(next.id);
          break;
        }
        case "k": {
          e.preventDefault();
          const idx = focusedNode ? visible.findIndex((n) => n.id === focusedNode.id) : -1;
          const prev = visible[Math.max(0, idx - 1)];
          if (prev) tree.focus(prev.id);
          break;
        }
        case "l": {
          e.preventDefault();
          if (!focusedNode) break;
          if (focusedNode.data.isDir) {
            tree.open(focusedNode.id);
          } else {
            focusedNode.activate();
          }
          break;
        }
        case "h": {
          e.preventDefault();
          if (!focusedNode) break;
          if (focusedNode.data.isDir && focusedNode.isOpen) {
            tree.close(focusedNode.id);
          } else if (focusedNode.parent && !focusedNode.parent.isRoot) {
            tree.focus(focusedNode.parent.id);
          }
          break;
        }
        case "g": {
          e.preventDefault();
          const now = Date.now();
          if (now - lastGRef.current < 400) {
            const first = visible[0];
            if (first) tree.focus(first.id);
            lastGRef.current = 0;
          } else {
            lastGRef.current = now;
          }
          break;
        }
        case "G": {
          e.preventDefault();
          const last = visible[visible.length - 1];
          if (last) tree.focus(last.id);
          break;
        }
        case "Enter": {
          if (!focusedNode) break;
          e.preventDefault();
          if (focusedNode.data.isDir) {
            focusedNode.toggle();
          } else {
            focusedNode.activate();
          }
          break;
        }
        default:
          break;
      }
    },
    [] // treeRef and lastGRef are refs — stable, no deps needed
  );

  if (rootLoading) {
    return (
      <div className={container}>
        <div className={loadingClass}>
          <span className={spinner} />
          Loading files…
        </div>
      </div>
    );
  }

  if (rootError) {
    return (
      <div className={container}>
        <div className={errorClass}>
          <span>⚠ {rootError}</span>
          <button
            className={retryButton}
            onClick={() => {
              setRootLoading(true);
              setRootError(null);
              fetchDirectoryFiles(sessionId, ".", includeIgnored, baseUrl)
                .then((response) => {
                  const nodes = (response.files || []).map(fileNodeToTreeNode);
                  setDirContents(new Map([[".", nodes]]));
                })
                .catch((err) => {
                  setRootError(err instanceof Error ? err.message : "Failed to load files");
                })
                .finally(() => setRootLoading(false));
            }}
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  // Search loading overlay.
  if (searchLoading) {
    return (
      <div className={container}>
        <div className={loadingClass}>
          <span className={spinner} />
          Searching…
        </div>
      </div>
    );
  }

  // Search empty state.
  if (searchResults !== null && searchResults.length === 0) {
    return (
      <div className={container}>
        <div className={searchEmpty}>No files match &ldquo;{searchTerm}&rdquo;</div>
      </div>
    );
  }

  if (treeData.length === 0 && searchResults === null) {
    return (
      <div className={container}>
        <div className={empty}>This directory is empty.</div>
      </div>
    );
  }

  return (
    <div
      className={container}
      ref={containerRef}
      tabIndex={0}
      onKeyDown={handleTreeKeyDown}
    >
      {searchTruncated && (
        <div className={searchTruncatedClass}>
          Showing first 500 results — refine your search for more specific matches.
        </div>
      )}
      <Tree<TreeNode>
        ref={treeRef}
        data={displayedData}
        idAccessor={(node) => node.id}
        childrenAccessor={(node) => {
          if (!node.isDir) return null;
          // Returning [] (not null) for all dirs keeps isLeaf=false so node.toggle() works.
          // Returning null would make the node a leaf and prevent any toggle from firing.
          return node.children ?? [];
        }}
        disableDrag={true}
        disableDrop={true}
        onActivate={handleActivate}
        onToggle={handleToggle}
        rowHeight={28}
        openByDefault={false}
        width={dims.w}
        height={dims.h}
        searchTerm={searchResults === null ? (searchTerm || undefined) : undefined}
        searchMatch={(node, term) => {
          const t = term.toLowerCase();
          return (
            node.data.name.toLowerCase().includes(t) ||
            node.data.id.toLowerCase().includes(t)
          );
        }}
      >
        {({ node, style, dragHandle }) => (
          <NodeRenderer
            node={node}
            style={style}
            dragHandle={dragHandle}
            gitStatusMap={gitStatusMap}
            dirStatusMap={dirStatusMap}
            loadingPaths={loadingPaths}
            errorPaths={errorPaths}
            selectedPath={selectedPath}
            includeIgnored={includeIgnored}
            searchTerm={searchTerm}
          />
        )}
      </Tree>
    </div>
  );
}
