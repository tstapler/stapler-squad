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
} from "lucide-react";
import * as styles from "./LocalFileBrowser.css";

interface FileEntry {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
}

interface DirListing {
  path: string;
  entries: FileEntry[];
  truncated?: boolean;
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

function serveUrl(absPath: string): string {
  return `/api/local/serve${absPath}`;
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
}

function ViewerToolbar({ name, url, openLabel = "Open in new tab" }: ViewerToolbarProps) {
  const handleOpen = () => window.open(url, "_blank", "noopener");
  return (
    <div className={styles.viewerToolbar}>
      <span className={styles.viewerLabel}>{name}</span>
      <button onClick={handleOpen} className={styles.externalButton} title={openLabel}>
        <ExternalLink size={14} />
      </button>
    </div>
  );
}

interface FileViewerProps {
  entry: FileEntry;
}

function FileViewer({ entry }: FileViewerProps) {
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
          <ViewerToolbar name={entry.name} url={url} />
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
          <ViewerToolbar name={entry.name} url={url} />
          {/* img tag sandboxes SVG — scripts in SVG don't execute in <img> context */}
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src={url} alt={entry.name} className={styles.viewerImage} />
        </div>
      );

    case "pdf":
      return (
        <div className={styles.viewerWrapper}>
          <ViewerToolbar name={entry.name} url={url} />
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
          <div className={styles.viewerToolbar}>
            <span className={styles.viewerLabel}>{entry.name}</span>
          </div>
          {/* eslint-disable-next-line jsx-a11y/media-has-caption */}
          <video controls src={url} className={styles.viewerVideo} />
        </div>
      );

    case "text":
      if (textError) {
        return <div className={styles.viewerEmpty}>Failed to load: {textError}</div>;
      }
      if (textContent === null) {
        return <div className={styles.viewerEmpty}>Loading…</div>;
      }
      return (
        <div className={styles.viewerWrapper}>
          <ViewerToolbar name={entry.name} url={url} openLabel="Open raw" />
          <pre className={styles.viewerText}>{textContent}</pre>
        </div>
      );

    default:
      return (
        <div className={styles.viewerEmpty}>
          <File size={32} />
          <span>{entry.name}</span>
          <span className={styles.viewerHint}>{formatSize(entry.size)} — binary file</span>
          <button
            onClick={() => window.open(url, "_blank", "noopener")}
            className={styles.externalButton}
          >
            Open / Download <ExternalLink size={14} />
          </button>
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

  const navigate = useCallback(
    (path: string) => {
      setCurrentPath(path);
      setPathInput(path);
      setSelectedEntry(null);
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
        return r.json() as Promise<DirListing>;
      })
      .then((data) => {
        setListing(data);
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

  const crumbs = buildBreadcrumbs(currentPath);

  return (
    <div className={styles.page}>
      <div className={styles.pathBar}>
        <form onSubmit={handlePathSubmit} className={styles.pathForm}>
          <input
            className={styles.pathInput}
            value={pathInput}
            onChange={(e) => setPathInput(e.target.value)}
            placeholder="/path/to/directory"
            aria-label="Directory path"
            spellCheck={false}
          />
          <button type="submit" className={styles.pathButton}>
            Go
          </button>
          <button
            type="button"
            className={styles.upButton}
            onClick={() => navigate(parentDir(currentPath))}
            disabled={currentPath === "/"}
            title="Go up one level"
          >
            <ArrowUp size={16} />
          </button>
        </form>
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
              >
                {crumb.label}
              </button>
            </span>
          ))}
        </nav>
      </div>

      <div className={styles.content}>
        <aside className={styles.sidebar}>
          {loading && <div className={styles.sidebarEmpty}>Loading…</div>}
          {listError && <div className={styles.sidebarEmpty}>{listError}</div>}
          {!loading && !listError && listing && listing.entries.length === 0 && (
            <div className={styles.sidebarEmpty}>Empty directory</div>
          )}
          {listing?.truncated && (
            <div className={styles.sidebarEmpty}>
              Showing first 5000 entries
            </div>
          )}
          {listing?.entries.map((entry) => {
            const Icon = fileIconForEntry(entry);
            const isActive = selectedEntry?.path === entry.path;
            return (
              <button
                key={entry.path}
                className={styles.fileEntry({ active: isActive })}
                onClick={() => handleEntryClick(entry)}
                title={entry.path}
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
            <FileViewer entry={selectedEntry} />
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
