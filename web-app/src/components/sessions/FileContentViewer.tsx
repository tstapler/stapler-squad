"use client";

import { useState, useEffect, useMemo, useRef } from "react";
import { useGetFileContent } from "@/lib/hooks/useFileService";
import { darkTheme } from "@/styles/theme.css";
import { buildGutterMarks, type GutterMarkType } from "@/lib/utils/parseDiff";
import {
  container, emptyState, emptyIcon, emptyHint,
  loading as loadingClass, error as errorClass, spinner,
  breadcrumb, breadcrumbSegment, breadcrumbCurrent, breadcrumbSep,
  truncationWarning, viewer, shikiOutput, shikiOutputWrap, plainPre, plainPreWrapped, codeMirrorEditor,
  binaryPlaceholder, binaryIcon, binaryTitle, binaryMeta,
  downloadButton, wrapToggleButton, wrapToggleButtonActive, imageViewer, imagePreview,
  pdfViewer, pdfEmbed,
  videoViewer, videoPlayer, videoMeta,
  shimmer,
  gutterMarkerAdd, gutterMarkerDelete, gutterMarkerModify,
} from "./FileContentViewer.css";

const GUTTER_MARKER_CLASS: Record<GutterMarkType, string> = {
  add: gutterMarkerAdd,
  delete: gutterMarkerDelete,
  modify: gutterMarkerModify,
};

// Language detection map: file extension → Shiki/CodeMirror language ID.
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

function detectLanguage(filePath: string): string {
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

const LARGE_FILE_LINE_THRESHOLD = 5000;

// ---- Shiki highlighter singleton ----

let highlighterPromise: Promise<import("shiki").Highlighter> | null = null;

async function getHighlighter() {
  if (!highlighterPromise) {
    const { getSingletonHighlighter } = await import("shiki");
    highlighterPromise = getSingletonHighlighter({
      themes: ["github-light", "github-dark"],
      langs: [],
    });
  }
  return highlighterPromise;
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

// ---- App theme hook ----

function useAppTheme(): "light" | "dark" {
  const [isDark, setIsDark] = useState<boolean>(() => {
    if (typeof document === "undefined") return true;
    return document.documentElement.classList.contains(darkTheme);
  });

  useEffect(() => {
    const observer = new MutationObserver(() => {
      setIsDark(document.documentElement.classList.contains(darkTheme));
    });
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["class"],
    });
    return () => observer.disconnect();
  }, []);

  return isDark ? "dark" : "light";
}

// ---- CodeMirror viewer (large files) ----

interface CodeMirrorViewerProps {
  content: string;
  language: string;
  wrapLines?: boolean;
  gutterMarks?: Map<number, GutterMarkType>;
}

function CodeMirrorViewer({ content, language, wrapLines, gutterMarks }: CodeMirrorViewerProps) {
  const editorRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<import("@codemirror/view").EditorView | null>(null);
  const appTheme = useAppTheme();
  const isDark = appTheme === "dark";

  useEffect(() => {
    let view: import("@codemirror/view").EditorView | null = null;

    (async () => {
      if (!editorRef.current) return;

      const { EditorView, gutter, GutterMarker } = await import("@codemirror/view");
      const { EditorState } = await import("@codemirror/state");
      const { basicSetup } = await import("codemirror");
      const { oneDark } = await import("@codemirror/theme-one-dark");

      // Load language extension.
      let langExtension = null;
      try {
        langExtension = await loadCodemirrorLang(language);
      } catch {
        // Fall back to plain text if language not supported.
      }

      class ChangeGutterMarker extends GutterMarker {
        constructor(private markType: GutterMarkType) {
          super();
        }
        eq(other: ChangeGutterMarker) {
          return other.markType === this.markType;
        }
        toDOM() {
          const el = document.createElement("div");
          el.className = GUTTER_MARKER_CLASS[this.markType];
          return el;
        }
      }

      const changeGutter = gutter({
        class: "cm-changeGutter",
        lineMarker(view, block) {
          if (!gutterMarks || gutterMarks.size === 0) return null;
          const lineNumber = view.state.doc.lineAt(block.from).number;
          const markType = gutterMarks.get(lineNumber);
          return markType ? new ChangeGutterMarker(markType) : null;
        },
      });

      // readOnly prevents edits; omitting editable.of(false) keeps contenteditable=true
      // so the browser allows text selection and copy.
      const extensions = [
        basicSetup,
        EditorState.readOnly.of(true),
        ...(isDark ? [oneDark] : []),
        ...(wrapLines ? [EditorView.lineWrapping] : []),
        ...(gutterMarks && gutterMarks.size > 0 ? [changeGutter] : []),
      ];
      if (langExtension) extensions.push(langExtension);

      const state = EditorState.create({
        doc: content,
        extensions,
      });

      view = new EditorView({ state, parent: editorRef.current });
      viewRef.current = view;
    })();

    return () => {
      view?.destroy();
      viewRef.current = null;
    };
  }, [content, language, isDark, wrapLines, gutterMarks]);

  return <div ref={editorRef} className={codeMirrorEditor} />;
}

async function loadCodemirrorLang(lang: string) {
  switch (lang) {
    case "javascript":
    case "jsx":
    case "typescript":
    case "tsx": {
      const { javascript } = await import("@codemirror/lang-javascript");
      const isTs = lang === "typescript" || lang === "tsx";
      const isJsx = lang === "jsx" || lang === "tsx";
      return javascript({ typescript: isTs, jsx: isJsx });
    }
    case "python": {
      const { python } = await import("@codemirror/lang-python");
      return python();
    }
    case "go": {
      const { go } = await import("@codemirror/lang-go");
      return go();
    }
    case "markdown": {
      const { markdown } = await import("@codemirror/lang-markdown");
      return markdown();
    }
    case "json": {
      const { json } = await import("@codemirror/lang-json");
      return json();
    }
    case "html": {
      const { html } = await import("@codemirror/lang-html");
      return html();
    }
    case "css":
    case "scss": {
      const { css } = await import("@codemirror/lang-css");
      return css();
    }
    case "rust": {
      const { rust } = await import("@codemirror/lang-rust");
      return rust();
    }
    case "java": {
      const { java } = await import("@codemirror/lang-java");
      return java();
    }
    default:
      return null;
  }
}

// ---- Shiki viewer (small/medium files) ----

interface ShikiViewerProps {
  content: string;
  language: string;
  wrapLines?: boolean;
}

function ShikiViewer({ content, language, wrapLines }: ShikiViewerProps) {
  const [html, setHtml] = useState<string | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;

    (async () => {
      try {
        const highlighter = await getHighlighter();
        // Ensure the language is loaded.
        try {
          await highlighter.loadLanguage(language as import("shiki").BundledLanguage);
        } catch {
          // Language might not exist in Shiki's bundle; fall back to plain text.
        }

        const result = highlighter.codeToHtml(content, {
          lang: language as import("shiki").BundledLanguage,
          themes: { light: "github-light", dark: "github-dark" },
        });

        if (!cancelled) setHtml(result);
      } catch (err) {
        console.error("Shiki highlighting error:", err);
        if (!cancelled) setError(true);
      }
    })();

    return () => { cancelled = true; };
  }, [content, language]);

  if (error) {
    // Error fallback — render plain text.
    return (
      <pre className={wrapLines ? plainPreWrapped : plainPre}>
        <code>{content}</code>
      </pre>
    );
  }

  if (html === null) {
    // Still loading — show shimmer to avoid flash of unstyled plain text.
    return <div className={shimmer} aria-hidden="true" />;
  }

  return (
    <div
      className={[shikiOutput, wrapLines ? shikiOutputWrap : ""].filter(Boolean).join(" ")}
      // Shiki generates safe HTML (no user content, only syntax highlights).
      dangerouslySetInnerHTML={{ __html: html }}
    />
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
}

export function FileContentViewer({ sessionId, filePath, baseUrl, diffContent }: FileContentViewerProps) {
  const { data, loading, error } = useGetFileContent(sessionId, filePath, baseUrl);
  const [wrapLines, setWrapLines] = useState(false);
  const gutterMarks = useMemo(
    () => (diffContent && filePath ? buildGutterMarks(diffContent, filePath) : new Map<number, GutterMarkType>()),
    [diffContent, filePath]
  );

  useEffect(() => {
    setWrapLines(false);
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
  const lineCount = (data.content.match(/\n/g) || []).length + 1;
  const hasGutterMarks = gutterMarks.size > 0;
  // Files with diff gutter markers use CodeMirror even below the line threshold — Shiki
  // (used for small/medium files) has no gutter decoration API.
  const useLargeMode = lineCount > LARGE_FILE_LINE_THRESHOLD || hasGutterMarks;

  return (
    <div className={container}>
      <Breadcrumb
        path={filePath}
        openUrl={openUrl}
        downloadUrl={downloadUrl}
        wrapLines={wrapLines}
        onToggleWrap={() => setWrapLines((w) => !w)}
      />
      {data.isTruncated && (
        <div className={truncationWarning}>
          ⚠ File truncated to 1 MB — only the first portion is shown
        </div>
      )}
      <div className={viewer}>
        {useLargeMode ? (
          <CodeMirrorViewer
            content={data.content}
            language={lang}
            wrapLines={wrapLines}
            gutterMarks={hasGutterMarks ? gutterMarks : undefined}
          />
        ) : (
          <ShikiViewer content={data.content} language={lang} wrapLines={wrapLines} />
        )}
      </div>
    </div>
  );
}
