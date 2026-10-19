"use client";

import { useState, useEffect, useMemo } from "react";
import { useGetFileContent } from "@/lib/hooks/useFileService";
import type { ReadViewHandle } from "./files/readViewHandle";
import type { BufferSource } from "@/lib/files/bufferReducer";
import { FallbackChip } from "./files/LargeFileStates";
import { WindowedFileView } from "./files/WindowedFileView";
import { ReadSurface } from "./files/ReadSurface";
import { buildGutterMarks, type GutterMarkType } from "@/lib/utils/parseDiff";
import {
  container, emptyState, emptyIcon, emptyHint,
  loading as loadingClass, error as errorClass, spinner,
  breadcrumb, breadcrumbSegment, breadcrumbCurrent, breadcrumbSep,
  truncationWarning, viewer,
  binaryPlaceholder, binaryIcon, binaryTitle, binaryMeta,
  downloadButton, wrapToggleButton, wrapToggleButtonActive, imageViewer, imagePreview,
  pdfViewer, pdfEmbed,
  videoViewer, videoPlayer, videoMeta,
} from "./FileContentViewer.css";

export { DOUBLE_G_WINDOW_MS, isDoubleGPress } from "./files/engine/doubleG";

// Language detection map: file extension → CodeMirror language ID.
const EXT_TO_LANG: Record<string, string> = {
  go: "go",
  ts: "typescript",
  tsx: "tsx",
  js: "javascript",
  jsx: "jsx",
  py: "python",
  rb: "ruby",
  rs: "rust",
  java: "java",
  kt: "kotlin",
  cs: "csharp",
  cpp: "cpp",
  cc: "cpp",
  c: "c",
  h: "c",
  hpp: "cpp",
  swift: "swift",
  php: "php",
  html: "html",
  htm: "html",
  css: "css",
  scss: "scss",
  sass: "sass",
  less: "less",
  json: "json",
  yaml: "yaml",
  yml: "yaml",
  toml: "toml",
  md: "markdown",
  markdown: "markdown",
  sh: "bash",
  bash: "bash",
  zsh: "bash",
  fish: "fish",
  sql: "sql",
  xml: "xml",
  graphql: "graphql",
  gql: "graphql",
  proto: "proto",
  tf: "hcl",
  hcl: "hcl",
  r: "r",
  lua: "lua",
  pl: "perl",
  ex: "elixir",
  exs: "elixir",
  erl: "erlang",
  hs: "haskell",
  clj: "clojure",
  dockerfile: "dockerfile",
  makefile: "makefile",
  mk: "makefile",
  diff: "diff",
  patch: "diff",
};

export function detectLanguage(filePath: string): string {
  const base = filePath.split("/").pop() || "";
  const lower = base.toLowerCase();

  // Check full filename first (Dockerfile, Makefile, etc.)
  if (lower === "dockerfile") return "dockerfile";
  if (lower === "makefile") return "makefile";
  if (lower === ".gitignore") return "ini";
  if (lower === ".env" || lower === ".envrc") return "ini";

  const ext = lower.split(".").pop() || "";
  return EXT_TO_LANG[ext] || "text";
}

// ---- Breadcrumb ----

interface BreadcrumbProps {
  path: string;
  onSegmentClick?: (path: string) => void;
  downloadUrl?: string;
  openUrl?: string;
  wrapLines?: boolean;
  onToggleWrap?: () => void;
}

function Breadcrumb({ path, onSegmentClick, downloadUrl, openUrl, wrapLines, onToggleWrap }: BreadcrumbProps) {
  const segments = path.split("/").filter(Boolean);
  return (
    <div className={breadcrumb}>
      {segments.map((seg, i) => {
        const segPath = segments.slice(0, i + 1).join("/");
        const isLast = i === segments.length - 1;
        return (
          <span key={segPath}>
            {isLast ? (
              <span className={breadcrumbCurrent} title={segPath}>
                {seg}
              </span>
            ) : (
              <button
                className={breadcrumbSegment}
                onClick={onSegmentClick ? () => onSegmentClick(segPath) : undefined}
                title={segPath}
                type="button"
              >
                {seg}
              </button>
            )}
            {!isLast && <span className={breadcrumbSep}>/</span>}
          </span>
        );
      })}
      {onToggleWrap && (
        <button
          className={[wrapToggleButton, wrapLines ? wrapToggleButtonActive : ""].filter(Boolean).join(" ")}
          onClick={onToggleWrap}
          title={wrapLines ? "Disable line wrap" : "Enable line wrap"}
        >
          ↵ Wrap
        </button>
      )}
      {openUrl && (
        <a
          href={openUrl}
          target="_blank"
          rel="noopener noreferrer"
          className={downloadButton}
          title="Open file in browser"
        >
          ↗ Open
        </a>
      )}
      {downloadUrl && (
        <a
          href={downloadUrl}
          download
          className={downloadButton}
          title="Download file"
        >
          ↓ Download
        </a>
      )}
    </div>
  );
}

// ---- Image content types ----

const IMAGE_CONTENT_TYPES = new Set([
  "image/png",
  "image/jpeg",
  "image/gif",
  "image/svg+xml",
  "image/webp",
  "image/bmp",
]);

const PDF_CONTENT_TYPES = new Set(["application/pdf"]);

const VIDEO_CONTENT_TYPES = new Set([
  "video/mp4",
  "video/webm",
  "video/quicktime",
  "video/ogg",
  "application/ogg",
]);

// Strip parameters (e.g. "; charset=utf-8") and normalize case before matching.
function matchesContentType(contentType: string | undefined, types: Set<string>): boolean {
  if (!contentType) return false;
  const base = contentType.split(";")[0].trim().toLowerCase();
  return types.has(base);
}

// ---- Main component ----

interface FileContentViewerProps {
  sessionId: string;
  filePath: string | null;
  baseUrl: string;
  /** Raw unified diff for the whole session — used to derive gutter markers for the open file. */
  diffContent?: string;
  /** Changes when the open file must be fetched again (stale-banner Reload). */
  reloadKey?: number;
  /** Called with each editor instance as it mounts, with the path it shows. */
  onEditorReady?: (handle: ReadViewHandle, path: string) => void;
  /** Show the plain paged Accessible view instead of CodeMirror (per-device choice). */
  accessible?: boolean;
  /** The buffer has unsaved edits (the Accessible view labels them). */
  dirty?: boolean;
  /** How the loaded buffer's text is read; absent until the open stat lands (the whole-file fetch starts meanwhile) and outside the buffered tab. */
  source?: BufferSource;
}

// +feature: file-content-viewer
export function FileContentViewer({ sessionId, filePath, baseUrl, diffContent, reloadKey, onEditorReady, accessible = false, dirty = false, source }: FileContentViewerProps) {
  const [demoted, setDemoted] = useState(false);
  const windowed = source?.kind === "windowed" && !demoted ? source : null;
  const { data, loading, error } = useGetFileContent(sessionId, windowed ? null : filePath, baseUrl, reloadKey);
  const [wrapLines, setWrapLines] = useState(false);
  const gutterMarks = useMemo(
    () => (diffContent && filePath ? buildGutterMarks(diffContent, filePath) : new Map<number, GutterMarkType>()),
    [diffContent, filePath]
  );

  useEffect(() => {
    setWrapLines(false);
    setDemoted(false);
  }, [filePath]);

  if (!filePath) {
    return (
      <div className={emptyState}>
        <span className={emptyIcon}>📄</span>
        <p>Select a file to view its contents</p>
        <p className={emptyHint}>Press ⌘P (or Ctrl+P) to quick-open any file</p>
      </div>
    );
  }

  const rawUrl = `/api/files/raw?sessionId=${encodeURIComponent(sessionId)}&path=${encodeURIComponent(filePath)}`;
  const downloadUrl = `${rawUrl}&download=true`;
  const openUrl = rawUrl;

  if (windowed) {
    return (
      <div className={container}>
        <Breadcrumb path={filePath} openUrl={openUrl} downloadUrl={downloadUrl} />
        <div className={viewer}>
          <WindowedFileView
            sessionId={sessionId}
            path={filePath}
            source={windowed}
            language={detectLanguage(filePath)}
            accessible={accessible}
            dirty={dirty}
            reloadKey={reloadKey}
            onReady={onEditorReady && ((handle) => onEditorReady(handle, filePath))}
            onDemoted={() => setDemoted(true)}
          />
        </div>
      </div>
    );
  }

  if (loading) {
    return (
      <div className={container}>
        <Breadcrumb path={filePath} />
        <div className={loadingClass}>
          <span className={spinner} />
          Loading file…
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className={container}>
        <Breadcrumb path={filePath} />
        <div className={errorClass}>
          <span>⚠ {error}</span>
        </div>
      </div>
    );
  }

  if (!data) return null;

  // Inline image rendering for known image content types.
  const isImage = data.isBinary && matchesContentType(data.contentType, IMAGE_CONTENT_TYPES);
  if (isImage) {
    return (
      <div className={container}>
        <Breadcrumb path={filePath} openUrl={openUrl} downloadUrl={downloadUrl} />
        <div className={imageViewer}>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={rawUrl}
            alt={filePath}
            className={imagePreview}
          />
        </div>
      </div>
    );
  }

  const isPdf = data.isBinary && matchesContentType(data.contentType, PDF_CONTENT_TYPES);
  if (isPdf) {
    return (
      <div className={container}>
        <Breadcrumb path={filePath} openUrl={openUrl} downloadUrl={downloadUrl} />
        <div className={pdfViewer}>
          <embed
            src={`${rawUrl}#view=FitH&navpanes=0`}
            type="application/pdf"
            className={pdfEmbed}
            title={filePath}
          />
        </div>
      </div>
    );
  }

  const isVideo = data.isBinary && matchesContentType(data.contentType, VIDEO_CONTENT_TYPES);
  if (isVideo) {
    const sizeKb = Number(data.size) / 1024;
    const sizeLabel = sizeKb >= 1024
      ? `${(sizeKb / 1024).toFixed(1)} MB`
      : `${sizeKb.toFixed(1)} KB`;

    return (
      <div className={container}>
        <Breadcrumb path={filePath} openUrl={openUrl} downloadUrl={downloadUrl} />
        <div className={videoViewer}>
          <video
            src={rawUrl}
            controls
            preload="metadata"
            className={videoPlayer}
          >
            Your browser does not support the video element.
          </video>
          <p className={videoMeta}>
            {filePath.split("/").pop()} · {sizeLabel}
            {data.contentType ? ` · ${data.contentType}` : ""}
          </p>
        </div>
      </div>
    );
  }

  if (data.isBinary) {
    const sizeKb = Number(data.size) / 1024;
    return (
      <div className={container}>
        <Breadcrumb path={filePath} openUrl={openUrl} downloadUrl={downloadUrl} />
        <div className={binaryPlaceholder}>
          <span className={binaryIcon}>🔒</span>
          <p className={binaryTitle}>Binary file — cannot display</p>
          <p className={binaryMeta}>
            {sizeKb >= 1024
              ? `${(sizeKb / 1024).toFixed(1)} MB`
              : `${sizeKb.toFixed(1)} KB`}
            {data.contentType ? ` · ${data.contentType}` : ""}
          </p>
        </div>
      </div>
    );
  }

  const lang = detectLanguage(filePath);
  const hasGutterMarks = gutterMarks.size > 0;

  return (
    <div className={container}>
      <Breadcrumb
        path={filePath}
        openUrl={openUrl}
        downloadUrl={downloadUrl}
        wrapLines={wrapLines}
        onToggleWrap={() => setWrapLines((w) => !w)}
      />
      {demoted && <FallbackChip />}
      {data.isTruncated && (
        <div className={truncationWarning}>
          ⚠ File truncated to 1 MB — only the first portion is shown
        </div>
      )}
      <div className={viewer}>
        <ReadSurface
          key={`${filePath}:${reloadKey ?? 0}:${accessible}`}
          accessible={accessible}
          dirty={dirty}
          content={data.content}
          language={lang}
          path={filePath}
          wrapLines={wrapLines}
          gutterMarks={hasGutterMarks ? gutterMarks : undefined}
          onReady={onEditorReady && ((handle) => onEditorReady(handle, filePath))}
        />
      </div>
    </div>
  );
}
