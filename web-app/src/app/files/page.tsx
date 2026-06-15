"use client";
// +feature: local-file-browser

import { useState, useEffect, useCallback } from "react";
import { usePageView } from "@/lib/analytics/usePageView";
import { getApiBaseUrl } from "@/lib/config";
import { FolderOpen, File, ChevronRight, Home, ArrowLeft } from "lucide-react";

interface FileEntry {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
}

interface DirectoryListing {
  path: string;
  parent: string;
  entries: FileEntry[];
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`;
}

function buildServeUrl(path: string): string {
  const origin = window.location.origin;
  // path is absolute (starts with /); omit the trailing slash on the prefix
  // so we get /api/local/serve/abs/path rather than a double-slash URL.
  return `${origin}/api/local/serve${path}`;
}

export default function FilesPage() {
  usePageView();
  const [listing, setListing] = useState<DirectoryListing | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const navigate = useCallback(async (path?: string) => {
    setLoading(true);
    setError(null);
    try {
      const base = getApiBaseUrl(); // e.g. http://localhost:8543/api
      const qs = path ? `?path=${encodeURIComponent(path)}` : "";
      const resp = await fetch(`${base}/local/files/list${qs}`);
      if (!resp.ok) {
        throw new Error((await resp.text()).trim() || `HTTP ${resp.status}`);
      }
      setListing(await resp.json() as DirectoryListing);
    } catch (e) {
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { void navigate(); }, [navigate]);

  const breadcrumbs = listing
    ? listing.path.split("/").filter(Boolean).reduce<{ label: string; path: string }[]>((acc, part, i, arr) => {
        acc.push({ label: part, path: "/" + arr.slice(0, i + 1).join("/") });
        return acc;
      }, [])
    : [];

  return (
    <div style={{ padding: "24px", maxWidth: "1200px", margin: "0 auto", fontFamily: "var(--font-mono, monospace)" }}>
      {/* Breadcrumb bar */}
      <div style={{ display: "flex", alignItems: "center", gap: "6px", marginBottom: "16px", fontSize: "13px" }}>
        <button
          onClick={() => void navigate()}
          title="Home directory"
          style={{ background: "none", border: "none", cursor: "pointer", padding: "4px", color: "var(--text-secondary)", display: "flex" }}
        >
          <Home size={15} />
        </button>
        {listing?.parent && (
          <button
            onClick={() => void navigate(listing.parent)}
            title="Parent directory"
            style={{ background: "none", border: "none", cursor: "pointer", padding: "4px", color: "var(--text-secondary)", display: "flex" }}
          >
            <ArrowLeft size={15} />
          </button>
        )}
        <span
          onClick={() => void navigate("/")}
          style={{ cursor: "pointer", color: "var(--primary)", userSelect: "none" }}
        >
          /
        </span>
        {breadcrumbs.map((crumb, i) => (
          <span key={crumb.path} style={{ display: "flex", alignItems: "center", gap: "6px" }}>
            <ChevronRight size={11} style={{ opacity: 0.4, flexShrink: 0 }} />
            <span
              onClick={() => void navigate(crumb.path)}
              style={{
                cursor: i < breadcrumbs.length - 1 ? "pointer" : "default",
                color: i < breadcrumbs.length - 1 ? "var(--primary)" : "var(--text-primary)",
                fontWeight: i === breadcrumbs.length - 1 ? 600 : 400,
                userSelect: "none",
              }}
            >
              {crumb.label}
            </span>
          </span>
        ))}
      </div>

      {error && (
        <div style={{
          padding: "10px 14px",
          background: "var(--error-bg)",
          color: "var(--error-text)",
          borderRadius: "6px",
          marginBottom: "16px",
          fontSize: "13px",
        }}>
          {error}
        </div>
      )}

      {loading ? (
        <div style={{ color: "var(--text-secondary)", fontSize: "13px", padding: "24px 0" }}>Loading…</div>
      ) : listing ? (
        <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "13px" }}>
          <thead>
            <tr style={{ borderBottom: "1px solid var(--border-color)" }}>
              <th style={{ padding: "8px 10px", fontWeight: 500, color: "var(--text-secondary)", textAlign: "left" }}>Name</th>
              <th style={{ padding: "8px 10px", fontWeight: 500, color: "var(--text-secondary)", textAlign: "right", width: "90px" }}>Size</th>
              <th style={{ padding: "8px 10px", fontWeight: 500, color: "var(--text-secondary)", textAlign: "left", width: "180px" }}>Modified</th>
            </tr>
          </thead>
          <tbody>
            {listing.entries.length === 0 && (
              <tr>
                <td colSpan={3} style={{ padding: "32px 10px", color: "var(--text-secondary)", textAlign: "center" }}>
                  Empty directory
                </td>
              </tr>
            )}
            {listing.entries.map((entry) => (
              <tr
                key={entry.path}
                onClick={() => {
                  if (entry.is_dir) {
                    void navigate(entry.path);
                  } else {
                    window.open(buildServeUrl(entry.path), "_blank", "noopener");
                  }
                }}
                style={{ borderBottom: "1px solid var(--border-color)", cursor: "pointer" }}
                onMouseEnter={(e) => { (e.currentTarget as HTMLTableRowElement).style.background = "var(--hover-background)"; }}
                onMouseLeave={(e) => { (e.currentTarget as HTMLTableRowElement).style.background = ""; }}
              >
                <td style={{ padding: "7px 10px" }}>
                  <div style={{ display: "flex", alignItems: "center", gap: "8px" }}>
                    {entry.is_dir
                      ? <FolderOpen size={14} style={{ color: "var(--primary)", flexShrink: 0 }} />
                      : <File size={14} style={{ color: "var(--text-secondary)", flexShrink: 0 }} />
                    }
                    <span style={{ color: entry.is_dir ? "var(--primary)" : "var(--text-primary)" }}>
                      {entry.name}
                    </span>
                  </div>
                </td>
                <td style={{ padding: "7px 10px", textAlign: "right", color: "var(--text-secondary)", fontVariantNumeric: "tabular-nums" }}>
                  {entry.is_dir ? "—" : formatSize(entry.size)}
                </td>
                <td style={{ padding: "7px 10px", color: "var(--text-secondary)" }}>
                  {new Date(entry.mod_time).toLocaleString()}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
    </div>
  );
}
