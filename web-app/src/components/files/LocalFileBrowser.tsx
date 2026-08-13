// +feature: local-file-browser
"use client";

import { useState, useEffect, useCallback } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import {
  Folder,
  File,
  FileText,
  FileImage,
  Globe,
  Film,
  ChevronRight,
  ArrowUp,
  ExternalLink,
  FolderOpen,
  X,
  TerminalSquare,
} from "lucide-react";
import { RepoPathInput } from "@/components/ui/RepoPathInput";
import { useSessionService } from "@/lib/hooks/useSessionService";
import { useAnalytics } from "@/lib/contexts/AnalyticsContext";
import { SessionType } from "@/gen/session/v1/types_pb";
import * as styles from "./LocalFileBrowser.css";

interface FileEntry {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
}

// Wire shape from GET /api/local/files/list — a plain REST JSON endpoint (not
// ConnectRPC), so field names are the Go struct's raw snake_case json tags.
interface RawFileEntry {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
}

interface RawDirListing {
  path: string;
  parent: string;
  entries: RawFileEntry[];
  total: number;
  has_more: boolean;
}

interface DirListing {
  path: string;
  entries: FileEntry[];
  total: number;
  hasMore: boolean;
}

function fromRawListing(raw: RawDirListing): DirListing {
  return {
    path: raw.path,
    total: raw.total,
    hasMore: raw.has_more,
    entries: raw.entries.map((e) => ({
      name: e.name,
      path: e.path,
      isDir: e.is_dir,
      size: e.size,
    })),
  };
}

type RenderMode = "html" | "image" | "svg" | "pdf" | "video" | "text" | "binary";

// Module-level constant — avoids allocating a new Set on every render/call.
const TEXT_EXTENSIONS = new Set([
  "txt", "md", "markdown", "rst", "log", "diff", "patch",
  "js", "mjs", "cjs", "ts", "tsx", "jsx",
  "css", "scss", "sass", "less",
  "json", "jsonc", "yaml", "yml", "toml", "ini", "env", "envrc", "conf", "cfg",
  "xml", "go", "py", "rb", "rs", "swift", "kt", "java", "scala",
  "c", "cpp", "cc", "cxx", "h", "hpp",
  "sh", "bash", "zsh", "fish", "bat", "cmd", "ps1",
  "sql", "graphql", "gql", "proto",
  "tf", "tfvars", "hcl", "mod", "sum", "lock",
  "dockerfile", "makefile", "mk",
  "gitignore", "gitattributes", "editorconfig",
  "php", "lua", "r", "pl", "pm",
]);

const MAX_TEXT_PREVIEW_BYTES = 2 * 1024 * 1024; // 2 MB

function inferRenderMode(filename: string): RenderMode {
  const ext = filename.split(".").pop()?.toLowerCase() ?? "";
  if (["html", "htm"].includes(ext)) return "html";
  if (ext === "svg") return "svg";
  if (["jpg", "jpeg", "png", "gif", "webp", "ico", "bmp", "avif", "tiff", "tif"].includes(ext))
    return "image";
  if (ext === "pdf") return "pdf";
  if (["mp4", "webm", "ogv", "mov", "avi"].includes(ext)) return "video";
  if (TEXT_EXTENSIONS.has(ext)) return "text";
  return "binary";
}

function fileIconForEntry(entry: FileEntry) {
  if (entry.isDir) return Folder;
  const mode = inferRenderMode(entry.name);
  switch (mode) {
    case "html": return Globe;
    case "svg":
    case "image": return FileImage;
    case "video": return Film;
    case "text": return FileText;
    default: return File;
  }
}

export function serveUrl(absPath: string): string {
  // Per-segment encoding so `#`, `?`, spaces, and unicode in filenames survive
  // as path characters instead of being read as a URL fragment/query or breaking.
  const encoded = absPath.split("/").map(encodeURIComponent).join("/");
  return `/api/local/serve${encoded}`;
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function parentDir(path: string): string {
  const cleaned = path.replace(/\/$/, "");
  const idx = cleaned.lastIndexOf("/");
  if (idx <= 0) return "/";
  return cleaned.slice(0, idx);
}

function buildBreadcrumbs(path: string): Array<{ label: string; path: string }> {
  const parts = path.split("/").filter(Boolean);
  const crumbs: Array<{ label: string; path: string }> = [{ label: "/", path: "/" }];
  for (let i = 0; i < parts.length; i++) {
    crumbs.push({ label: parts[i], path: "/" + parts.slice(0, i + 1).join("/") });
  }
  return crumbs;
}

interface ViewerToolbarProps {
  name: string;
  url: string;
  openLabel?: string;
  onClose?: () => void;
}

function ViewerToolbar({ name, url, openLabel = "Open in new tab", onClose }: ViewerToolbarProps) {
  const handleOpen = () => window.open(url, "_blank", "noopener");
  return (
    <div className={styles.viewerToolbar}>
      <span className={styles.viewerLabel}>{name}</span>
      <button onClick={handleOpen} className={styles.externalButton} title={openLabel} data-testid="file-browser-viewer-open-external">
        <ExternalLink size={14} />
      </button>
      {onClose && (
        <button onClick={onClose} className={styles.externalButton} title="Close file viewer" data-testid="file-browser-viewer-close">
          <X size={14} />
        </button>
      )}
    </div>
  );
}

interface FileViewerProps {
  entry: FileEntry;
  onClose: () => void;
}

function FileViewer({ entry, onClose }: FileViewerProps) {
  const [textContent, setTextContent] = useState<string | null>(null);
  const [textError, setTextError] = useState<string | null>(null);
  const url = serveUrl(entry.path);
  const mode = inferRenderMode(entry.name);

  useEffect(() => {
    if (mode !== "text") {
      setTextContent(null);
      setTextError(null);
      return;
    }
    if (entry.size > MAX_TEXT_PREVIEW_BYTES) {
      setTextError(`File too large to preview (${formatSize(entry.size)}) — open externally`);
      return;
    }
    const controller = new AbortController();
    setTextContent(null);
    setTextError(null);
    fetch(url, { signal: controller.signal })
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        return r.text();
      })
      .then(setTextContent)
      .catch((e: unknown) => {
        if (e instanceof Error && e.name === "AbortError") return;
        setTextError(e instanceof Error ? e.message : String(e));
      });
    return () => controller.abort();
  }, [url, mode, entry.size]);

  switch (mode) {
    case "html":
      return (
        <div className={styles.viewerWrapper}>
          <ViewerToolbar name={entry.name} url={url} onClose={onClose} />
          <iframe
            src={url}
            className={styles.viewerFrame}
            sandbox="allow-scripts allow-forms allow-popups allow-modals allow-downloads"
            title={entry.name}
          />
        </div>
      );

    case "svg":
    case "image":
      return (
        <div className={styles.viewerWrapper}>
          <ViewerToolbar name={entry.name} url={url} onClose={onClose} />
          {/* img tag sandboxes SVG — scripts in SVG don't execute in <img> context */}
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src={url} alt={entry.name} className={styles.viewerImage} />
        </div>
      );

    case "pdf":
      return (
        <div className={styles.viewerWrapper}>
          <ViewerToolbar name={entry.name} url={url} onClose={onClose} />
          <iframe
            src={url}
            className={styles.viewerFrame}
            sandbox="allow-scripts allow-forms allow-popups"
            title={entry.name}
          />
        </div>
      );

    case "video":
      return (
        <div className={styles.viewerWrapper}>
          <ViewerToolbar name={entry.name} url={url} onClose={onClose} />
          {/* eslint-disable-next-line jsx-a11y/media-has-caption */}
          <video controls src={url} className={styles.viewerVideo} />
        </div>
      );

    case "text":
      if (textError) {
        return (
          <div className={styles.viewerWrapper}>
            <ViewerToolbar name={entry.name} url={url} onClose={onClose} />
            <div className={styles.viewerEmpty}>Failed to load: {textError}</div>
          </div>
        );
      }
      if (textContent === null) {
        return (
          <div className={styles.viewerWrapper}>
            <ViewerToolbar name={entry.name} url={url} onClose={onClose} />
            <div className={styles.viewerEmpty}>Loading…</div>
          </div>
        );
      }
      return (
        <div className={styles.viewerWrapper}>
          <ViewerToolbar name={entry.name} url={url} openLabel="Open raw" onClose={onClose} />
          <pre className={styles.viewerText}>{textContent}</pre>
        </div>
      );

    default:
      return (
        <div className={styles.viewerWrapper}>
          <ViewerToolbar name={entry.name} url={url} onClose={onClose} />
          <div className={styles.viewerEmpty}>
            <File size={32} />
            <span>{entry.name}</span>
            <span className={styles.viewerHint}>{formatSize(entry.size)} — binary file</span>
            <button
              onClick={() => window.open(url, "_blank", "noopener")}
              className={styles.externalButton}
              data-testid="file-browser-viewer-download"
            >
              Open / Download <ExternalLink size={14} />
            </button>
          </div>
        </div>
      );
  }
}

export function LocalFileBrowser() {
  const searchParams = useSearchParams();
  const router = useRouter();

  const initialPath = searchParams.get("path") ?? "/";
  const [pathInput, setPathInput] = useState(initialPath);
  const [currentPath, setCurrentPath] = useState(initialPath);
  const [listing, setListing] = useState<DirListing | null>(null);
  const [listError, setListError] = useState<string | null>(null);
  const [selectedEntry, setSelectedEntry] = useState<FileEntry | null>(null);
  const [loading, setLoading] = useState(false);
  const [filterText, setFilterText] = useState("");
  const [terminalError, setTerminalError] = useState<string | null>(null);
  const [openingTerminal, setOpeningTerminal] = useState(false);
  const { createSession } = useSessionService({ enabled: true });
  const analytics = useAnalytics();

  const navigate = useCallback(
    (path: string) => {
      setCurrentPath(path);
      setPathInput(path);
      setSelectedEntry(null);
      setFilterText("");
      router.replace(`/files?path=${encodeURIComponent(path)}`);
    },
    [router]
  );

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setListError(null);
    setListing(null);
    fetch(`/api/local/files/list?path=${encodeURIComponent(currentPath)}`, {
      signal: controller.signal,
    })
      .then((r) => {
        if (!r.ok)
          return r.text().then((t) => {
            throw new Error((t || `HTTP ${r.status}`).trim());
          });
        return r.json() as Promise<RawDirListing>;
      })
      .then((data) => {
        setListing(fromRawListing(data));
      })
      .catch((e: unknown) => {
        if (e instanceof Error && e.name === "AbortError") return;
        setListError(e instanceof Error ? e.message : String(e));
      })
      .finally(() => setLoading(false));
    return () => controller.abort();
  }, [currentPath]);

  const handlePathSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    navigate(pathInput.trim() || "/");
  };

  const handleEntryClick = (entry: FileEntry) => {
    if (entry.isDir) {
      navigate(entry.path);
    } else {
      setSelectedEntry(entry);
    }
  };

  const handleOpenTerminal = async () => {
    setTerminalError(null);
    setOpeningTerminal(true);
    try {
      const session = await createSession({ path: currentPath, sessionType: SessionType.DIRECTORY });
      analytics.track({ name: "file_browser.open_terminal", category: "user_action", component: "LocalFileBrowser" });
      if (session) {
        router.push(`/?session=${session.id}`);
      } else {
        setTerminalError("Failed to create session");
      }
    } catch (e: unknown) {
      setTerminalError(e instanceof Error ? e.message : String(e));
    } finally {
      setOpeningTerminal(false);
    }
  };

  const crumbs = buildBreadcrumbs(currentPath);
  const filteredEntries = (listing?.entries ?? []).filter((entry) =>
    entry.name.toLowerCase().includes(filterText.trim().toLowerCase())
  );

  return (
    <div className={styles.page}>
      <div className={styles.pathBar}>
        <form onSubmit={handlePathSubmit} className={styles.pathForm}>
          <div className={styles.pathInputWrapper}>
            <RepoPathInput
              value={pathInput}
              onChange={setPathInput}
              onSelect={(entry) => navigate(entry.path)}
              placeholder="/path/to/directory"
              data-testid="file-browser-path-input"
            />
          </div>
          <button type="submit" className={styles.pathButton} data-testid="file-browser-go-button">
            Go
          </button>
          <button
            type="button"
            className={styles.upButton}
            onClick={() => navigate(parentDir(currentPath))}
            disabled={currentPath === "/"}
            title="Go up one level"
            data-testid="file-browser-up-button"
          >
            <ArrowUp size={16} />
          </button>
          <button
            type="button"
            className={styles.upButton}
            onClick={handleOpenTerminal}
            disabled={openingTerminal}
            title="Open terminal here"
            data-testid="file-browser-open-terminal"
          >
            <TerminalSquare size={16} />
          </button>
        </form>
        {terminalError && <div className={styles.sidebarEmpty}>{terminalError}</div>}
        <nav className={styles.breadcrumbs} aria-label="Path breadcrumbs">
          {crumbs.map((crumb, i) => (
            <span key={crumb.path} className={styles.crumbGroup}>
              {i > 0 && (
                <span className={styles.breadcrumbSep} aria-hidden="true">
                  <ChevronRight size={12} />
                </span>
              )}
              <button
                className={styles.breadcrumbLink}
                onClick={() => navigate(crumb.path)}
                data-testid={`file-browser-breadcrumb-${i}`}
              >
                {crumb.label}
              </button>
            </span>
          ))}
        </nav>
        <input
          className={styles.pathInput}
          value={filterText}
          onChange={(e) => setFilterText(e.target.value)}
          placeholder="Filter entries…"
          aria-label="Filter entries"
          spellCheck={false}
          data-testid="file-browser-filter-input"
        />
      </div>

      <div className={styles.content({ fileOpen: !!selectedEntry })}>
        <aside className={styles.sidebar} data-testid="file-browser-entry-list">
          {loading && <div className={styles.sidebarEmpty}>Loading…</div>}
          {listError && <div className={styles.sidebarEmpty}>{listError}</div>}
          {!loading && !listError && listing && listing.entries.length === 0 && (
            <div className={styles.sidebarEmpty}>Empty directory</div>
          )}
          {!loading && !listError && listing && listing.entries.length > 0 && filteredEntries.length === 0 && (
            <div className={styles.sidebarEmpty}>No matching entries</div>
          )}
          {listing?.hasMore && (
            <div className={styles.sidebarEmpty} data-testid="file-browser-truncation-notice">
              Showing first {listing.entries.length} of {listing.total} entries
            </div>
          )}
          {filteredEntries.map((entry) => {
            const Icon = fileIconForEntry(entry);
            const isActive = selectedEntry?.path === entry.path;
            return (
              <button
                key={entry.path}
                className={styles.fileEntry({ active: isActive })}
                onClick={() => handleEntryClick(entry)}
                title={entry.path}
                data-testid="file-browser-entry"
              >
                <span className={styles.fileIcon}>
                  <Icon size={15} />
                </span>
                <span className={styles.fileName}>
                  {entry.name}
                  {entry.isDir ? "/" : ""}
                </span>
                {!entry.isDir && (
                  <span className={styles.fileSize}>{formatSize(entry.size)}</span>
                )}
              </button>
            );
          })}
        </aside>

        <div className={styles.viewer}>
          {selectedEntry ? (
            <FileViewer entry={selectedEntry} onClose={() => setSelectedEntry(null)} />
          ) : (
            <div className={styles.viewerEmpty}>
              <FolderOpen size={40} />
              <span>Select a file to view</span>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
