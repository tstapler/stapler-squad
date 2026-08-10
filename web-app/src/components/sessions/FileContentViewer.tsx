"use client";

import { useState, useEffect, useMemo, useRef } from "react";
import { useGetFileContent } from "@/lib/hooks/useFileService";
import { vars } from "@/styles/theme.css";
import { buildGutterMarks, type GutterMarkType } from "@/lib/utils/parseDiff";
import {
  container, emptyState, emptyIcon, emptyHint,
  loading as loadingClass, error as errorClass, spinner,
  breadcrumb, breadcrumbSegment, breadcrumbCurrent, breadcrumbSep,
  truncationWarning, viewer, codeMirrorEditor,
  binaryPlaceholder, binaryIcon, binaryTitle, binaryMeta,
  downloadButton, wrapToggleButton, wrapToggleButtonActive, imageViewer, imagePreview,
  pdfViewer, pdfEmbed,
  videoViewer, videoPlayer, videoMeta,
  gutterMarkerAdd, gutterMarkerDelete, gutterMarkerModify,
} from "./FileContentViewer.css";

const GUTTER_MARKER_CLASS: Record<GutterMarkType, string> = {
  add: gutterMarkerAdd,
  delete: gutterMarkerDelete,
  modify: gutterMarkerModify,
};

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

// ---- CodeMirror viewer ----
//
// Editor chrome and syntax colors are sourced from theme tokens (mainly the
// `terminal*` family, matching FilesTab.css.ts's container) rather than a
// hardcoded dark theme. These tokens resolve to CSS custom properties scoped
// by the active theme class on <html>, so no JS theme-detection is needed —
// switching themes (including non-dark ones like "clean") repaints automatically.

interface CodeMirrorViewerProps {
  content: string;
  language: string;
  wrapLines?: boolean;
  gutterMarks?: Map<number, GutterMarkType>;
}

function CodeMirrorViewer({ content, language, wrapLines, gutterMarks }: CodeMirrorViewerProps) {
  const editorRef = useRef<HTMLDivElement>(null);
  const viewRef = useRef<import("@codemirror/view").EditorView | null>(null);

  useEffect(() => {
    let view: import("@codemirror/view").EditorView | null = null;

    (async () => {
      if (!editorRef.current) return;

      const { EditorView, gutter, GutterMarker, keymap } = await import("@codemirror/view");
      const { EditorState } = await import("@codemirror/state");
      const { basicSetup } = await import("codemirror");
      const { HighlightStyle, syntaxHighlighting } = await import("@codemirror/language");
      const { tags } = await import("@lezer/highlight");
      const {
        cursorCharLeft, cursorCharRight, cursorLineDown, cursorLineUp,
        selectCharLeft, selectCharRight, selectLineDown, selectLineUp,
        cursorLineBoundaryBackward, cursorLineBoundaryForward,
        selectLineBoundaryBackward, selectLineBoundaryForward,
        cursorDocStart, cursorDocEnd, cursorPageDown, cursorPageUp,
        selectPageDown, selectPageUp,
      } = await import("@codemirror/commands");
      const { openSearchPanel, findNext, findPrevious } = await import("@codemirror/search");

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

      // Editor chrome (background/foreground/gutters/selection/cursor) sourced from the
      // theme-invariant terminal token family, matching FilesTab.css.ts's container.
      const cmTheme = EditorView.theme(
        {
          "&": {
            color: vars.color.terminalForeground,
            backgroundColor: vars.color.terminalBackground,
            height: "100%",
          },
          ".cm-content": {
            caretColor: vars.color.terminalCursor,
          },
          ".cm-cursor, .cm-dropCursor": {
            borderLeftColor: vars.color.terminalCursor,
          },
          "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection": {
            backgroundColor: `${vars.color.terminalHoverBg} !important`,
          },
          ".cm-activeLine": {
            backgroundColor: vars.color.terminalHoverBg,
          },
          ".cm-activeLineGutter": {
            backgroundColor: vars.color.terminalHoverBg,
          },
          ".cm-gutters": {
            backgroundColor: vars.color.terminalBackground,
            color: vars.color.terminalTextMuted,
            borderRight: `1px solid ${vars.color.terminalBorder}`,
          },
          ".cm-lineNumbers .cm-gutterElement": {
            color: vars.color.terminalTextMuted,
          },
        },
        { dark: true },
      );

      // Syntax-token colors, sourced from existing semantic tokens so they stay
      // consistent with the rest of the app's accent/status palette.
      const cmHighlightStyle = HighlightStyle.define([
        { tag: [tags.keyword, tags.controlKeyword, tags.operatorKeyword, tags.modifier], color: vars.color.primary },
        { tag: [tags.string, tags.special(tags.string)], color: vars.color.success },
        { tag: [tags.comment, tags.lineComment, tags.blockComment], color: vars.color.terminalTextMuted, fontStyle: "italic" },
        { tag: [tags.number, tags.bool, tags.null], color: vars.color.warning },
        { tag: [tags.function(tags.variableName), tags.function(tags.propertyName)], color: vars.color.accentText },
        { tag: [tags.definition(tags.variableName), tags.definition(tags.propertyName)], color: vars.color.terminalForeground },
        { tag: [tags.className, tags.typeName], color: vars.color.primary },
        { tag: [tags.tagName, tags.attributeName], color: vars.color.gitModified },
        { tag: tags.invalid, color: vars.color.error },
      ]);

      // Vim-style navigation/selection, matching FileTree.tsx's handleTreeKeyDown
      // semantics: hjkl movement, gg/G doc start/end, ctrl-d/u paging, v-extended
      // selection via shift, y to yank, / n N for search.
      let lastGPress = 0;
      const vimKeymap = keymap.of([
        { key: "h", run: cursorCharLeft },
        { key: "l", run: cursorCharRight },
        { key: "j", run: cursorLineDown },
        { key: "k", run: cursorLineUp },
        { key: "0", run: cursorLineBoundaryBackward },
        { key: "$", run: cursorLineBoundaryForward },
        {
          key: "g",
          run: (v) => {
            const now = Date.now();
            const isDoubleG = now - lastGPress < 400;
            lastGPress = isDoubleG ? 0 : now;
            return isDoubleG ? cursorDocStart(v) : true;
          },
        },
        { key: "G", run: cursorDocEnd },
        { key: "Ctrl-d", run: cursorPageDown },
        { key: "Ctrl-u", run: cursorPageUp },
        { key: "Shift-h", run: selectCharLeft },
        { key: "Shift-l", run: selectCharRight },
        { key: "Shift-j", run: selectLineDown },
        { key: "Shift-k", run: selectLineUp },
        { key: "Shift-0", run: selectLineBoundaryBackward },
        { key: "Shift-$", run: selectLineBoundaryForward },
        { key: "Shift-Ctrl-d", run: selectPageDown },
        { key: "Shift-Ctrl-u", run: selectPageUp },
        {
          key: "y",
          run: (v) => {
            const selected = v.state.sliceDoc(v.state.selection.main.from, v.state.selection.main.to);
            if (selected) void navigator.clipboard.writeText(selected);
            return true;
          },
        },
        { key: "/", run: openSearchPanel },
        { key: "n", run: findNext },
        { key: "Shift-n", run: findPrevious },
      ]);

      // readOnly prevents edits; omitting editable.of(false) keeps contenteditable=true
      // so the browser allows text selection and copy.
      const extensions = [
        basicSetup,
        EditorState.readOnly.of(true),
        cmTheme,
        syntaxHighlighting(cmHighlightStyle),
        vimKeymap,
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
  }, [content, language, wrapLines, gutterMarks]);

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

// +feature: file-content-viewer
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
      {data.isTruncated && (
        <div className={truncationWarning}>
          ⚠ File truncated to 1 MB — only the first portion is shown
        </div>
      )}
      <div className={viewer}>
        <CodeMirrorViewer
          content={data.content}
          language={lang}
          wrapLines={wrapLines}
          gutterMarks={hasGutterMarks ? gutterMarks : undefined}
        />
      </div>
    </div>
  );
}
