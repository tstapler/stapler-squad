"use client";

import { useState, useEffect, useRef } from "react";
import { useGetFileContent } from "@/lib/hooks/useFileService";
import { darkTheme } from "@/styles/theme.css";
import {
  container, emptyState, emptyIcon,
  loading as loadingClass, error as errorClass, spinner,
  breadcrumb, breadcrumbSegment, breadcrumbCurrent, breadcrumbSep,
  truncationWarning, viewer, shikiOutput, plainPre, codeMirrorEditor,
  binaryPlaceholder, binaryIcon, binaryTitle, binaryMeta,
  downloadButton, imageViewer, imagePreview,
} from "./FileContentViewer.css";

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
}

function Breadcrumb({ path, onSegmentClick, downloadUrl }: BreadcrumbProps) {
  const segments = path.split("/").filter(Boolean);
  return (
    <div className={breadcrumb}>
      {segments.map((seg, i) => {
        const segPath = segments.slice(0, i + 1).join("/");
        const isLast = i === segments.length - 1;
        return (
          <span key={segPath}>
            <span
              className={isLast ? breadcrumbCurrent : breadcrumbSegment}
              onClick={!isLast && onSegmentClick ? () => onSegmentClick(segPath) : undefined}
              title={segPath}
            >
              {seg}
            </span>
            {!isLast && <span className={breadcrumbSep}>/</span>}
          </span>
        );
      })}
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
}

function CodeMirrorViewer({ content, language }: CodeMirrorViewerProps) {
  const editorRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<import("@codemirror/view").EditorView | null>(null);
  const appTheme = useAppTheme();
  const isDark = appTheme === "dark";

  useEffect(() => {
    let view: import("@codemirror/view").EditorView | null = null;

    (async () => {
      if (!editorRef.current) return;

      const { EditorView } = await import("@codemirror/view");
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

      const extensions = [
        basicSetup,
        EditorState.readOnly.of(true),
        EditorView.editable.of(false),
        ...(isDark ? [oneDark] : []),
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
  }, [content, language, isDark]);

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
}

function ShikiViewer({ content, language }: ShikiViewerProps) {
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

  if (error || html === null) {
    // Plain text fallback.
    return (
      <pre className={plainPre}>
        <code>{content}</code>
      </pre>
    );
  }

  return (
    <div
      className={shikiOutput}
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

// ---- Main component ----

interface FileContentViewerProps {
  sessionId: string;
  filePath: string | null;
  baseUrl: string;
}

export function FileContentViewer({ sessionId, filePath, baseUrl }: FileContentViewerProps) {
  const { data, loading, error } = useGetFileContent(sessionId, filePath, baseUrl);

  if (!filePath) {
    return (
      <div className={emptyState}>
        <span className={emptyIcon}>📄</span>
        <p>Select a file to view its contents</p>
      </div>
    );
  }

  const rawUrl = `/api/files/raw?sessionId=${encodeURIComponent(sessionId)}&path=${encodeURIComponent(filePath)}`;
  const downloadUrl = `${rawUrl}&download=true`;

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
  const isImage = data.isBinary && IMAGE_CONTENT_TYPES.has(data.contentType ?? "");
  if (isImage) {
    return (
      <div className={container}>
        <Breadcrumb path={filePath} downloadUrl={downloadUrl} />
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

  if (data.isBinary) {
    const sizeKb = Number(data.size) / 1024;
    return (
      <div className={container}>
        <Breadcrumb path={filePath} downloadUrl={downloadUrl} />
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
  const useLargeMode = lineCount > LARGE_FILE_LINE_THRESHOLD;

  return (
    <div className={container}>
      <Breadcrumb path={filePath} downloadUrl={downloadUrl} />
      {data.isTruncated && (
        <div className={truncationWarning}>
          ⚠ File truncated to 1 MB — only the first portion is shown
        </div>
      )}
      <div className={viewer}>
        {useLargeMode ? (
          <CodeMirrorViewer content={data.content} language={lang} />
        ) : (
          <ShikiViewer content={data.content} language={lang} />
        )}
      </div>
    </div>
  );
}
