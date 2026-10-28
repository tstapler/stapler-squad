"use client";

import { Fragment, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { parseDiff, type DiffFile, type DiffHunk, type DiffLine } from "@/lib/utils/parseDiff";
import { buildHunkList, hunkDomId, pairSplitRows } from "@/lib/diff/diffNavigation";
import type { Highlighter, TokenSpan } from "@/lib/highlight/types";
import { CONTEXT_SERVER_DEFAULT, CONTEXT_WHOLE_FILE, DEFAULT_CONTEXT_LINES } from "@/lib/diff/contextLines";
import {
  container,
  toolbar,
  stats,
  filesChanged,
  additions,
  deletions,
  viewModeToggle,
  viewModeButton,
  viewModeButtonActive,
  body,
  fileTree,
  fileTreeItem,
  fileTreeItemActive,
  fileTreeFilename,
  fileTreeStats,
  diffContent,
  file,
  fileHeader,
  filename,
  fileStats,
  hunk,
  hunkHeader,
  lines,
  line,
  lineAdd,
  lineDelete,
  lineContext,
  lineNumber,
  lineContent,
  loading as loadingClass,
  empty as emptyClass,
  emptyHint,
  errorState,
  errorRetryButton,
  hunkActive,
  toolbarActions,
  hunkNavButtons,
  hunkNavButton,
  hunkCounter,
  expandRow,
  expandButton,
  splitGrid,
  splitNumber,
  splitContent,
  splitRightBorder,
  splitAdd,
  splitDelete,
  splitPlaceholder,
  syntaxScope,
} from "./DiffRenderer.css";

export type DiffViewMode = "unified" | "split";


/** Lines revealed per "Show N more lines" click. */
export const EXPAND_STEP = 10;

export interface DiffRendererProps {
  /** Raw unified diff string from git. */
  content: string;
  added: number;
  removed: number;
  loading?: boolean;
  /**
   * Set when the diff fetch itself failed (as opposed to succeeding with a
   * genuinely empty diff). Rendered as a distinct error state with a retry
   * button instead of the ambiguous "No changes to display" empty state.
   */
  error?: string | null;
  /** Called when the user clicks the refresh button, or the error state's retry button. */
  onRefresh?: () => void;
  /** Controlled view mode; when `onViewModeChange` is omitted the renderer keeps its own. */
  viewMode?: DiffViewMode;
  onViewModeChange?: (mode: DiffViewMode) => void;
  /** DOM id (see `hunkDomId`) of the hunk to highlight and scroll into view. */
  activeHunkId?: string | null;
  onPrevHunk?: () => void;
  onNextHunk?: () => void;
  /** Syntax highlighter; with `languageForFile` enables token colors. Lines render plain until tokenizing finishes. */
  highlighter?: Highlighter;
  languageForFile?: (filename: string) => string;
  /**
   * Context lines currently in `content` (0 = server default, -1 = whole file).
   * Providing `onContextLinesChange` enables the expand controls.
   */
  contextLines?: number;
  onContextLinesChange?: (contextLines: number) => void;
}

interface HunkTokens {
  /** Offset of each line's text (prefix stripped) within the tokenized hunk text. */
  lineStarts: number[];
  /** Spans overlapping each line, bucketed once so rendering a line doesn't rescan the hunk's spans. */
  spansByLine: TokenSpan[][];
}

/** Index of the last line whose start is <= offset (lineStarts is ascending). */
function lineIndexAt(lineStarts: number[], offset: number): number {
  let lo = 0;
  let hi = lineStarts.length - 1;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (lineStarts[mid] <= offset) lo = mid;
    else hi = mid - 1;
  }
  return lo;
}

function bucketSpansByLine(spans: TokenSpan[], lineStarts: number[]): TokenSpan[][] {
  const buckets: TokenSpan[][] = lineStarts.map(() => []);
  for (const span of spans) {
    if (span.to <= span.from) continue;
    for (let li = lineIndexAt(lineStarts, span.from); li < lineStarts.length && lineStarts[li] < span.to; li++) {
      buckets[li].push(span);
    }
  }
  return buckets;
}

/** Hunks tokenized between re-renders. */
const TOKEN_BATCH = 8;

const hunkKey = (fi: number, hi: number) => `${fi}:${hi}`;

/** Splits one line's text into highlighted spans using the hunk-level token offsets. */
function renderTokens(text: string, lineStart: number, spans: TokenSpan[]): ReactNode {
  const end = lineStart + text.length;
  const out: ReactNode[] = [];
  let cursor = lineStart;
  for (const span of spans) {
    if (span.to <= lineStart || span.from >= end) continue;
    const from = Math.max(span.from, lineStart);
    const to = Math.min(span.to, end);
    if (from > cursor) out.push(text.slice(cursor - lineStart, from - lineStart));
    out.push(
      <span key={from} className={span.className}>
        {text.slice(from - lineStart, to - lineStart)}
      </span>,
    );
    cursor = to;
  }
  if (cursor < end) out.push(text.slice(cursor - lineStart));
  return out;
}

/** Tokenizes each hunk of every file in turn, yielding to the event loop between hunks. */
function useHunkTokens(files: DiffFile[], highlighter?: Highlighter, languageForFile?: (f: string) => string) {
  const [tokens, setTokens] = useState<Map<string, HunkTokens>>(new Map());

  useEffect(() => {
    setTokens((prev) => (prev.size ? new Map() : prev));
    if (!highlighter || !languageForFile) return;
    let cancelled = false;
    // Publish in batches: a state update per hunk re-renders the whole diff each time.
    const acc = new Map<string, HunkTokens>();
    let unpublished = 0;
    const publish = () => {
      if (unpublished === 0) return;
      unpublished = 0;
      setTokens(new Map(acc));
    };
    (async () => {
      for (let fi = 0; fi < files.length && !cancelled; fi++) {
        const languageId = languageForFile(files[fi].filename);
        for (let hi = 0; hi < files[fi].changes.length && !cancelled; hi++) {
          const hunkLines = files[fi].changes[hi].lines;
          const lineStarts: number[] = [];
          let offset = 0;
          const text = hunkLines
            .map((l) => {
              lineStarts.push(offset);
              const t = l.content.slice(1);
              offset += t.length + 1;
              return t;
            })
            .join("\n");
          let spans: TokenSpan[] = [];
          try {
            spans = await highlighter.tokenize({ startLine: 1, endLine: hunkLines.length, text }, languageId);
          } catch {
            // Highlighting is cosmetic; leave the hunk plain.
          }
          if (cancelled) return;
          if (spans.length > 0) {
            acc.set(hunkKey(fi, hi), { lineStarts, spansByLine: bucketSpansByLine(spans, lineStarts) });
            if (++unpublished >= TOKEN_BATCH) publish();
          }
          await new Promise((resolve) => setTimeout(resolve, 0));
        }
      }
      if (!cancelled) publish();
    })();
    return () => {
      cancelled = true;
    };
  }, [files, highlighter, languageForFile]);

  return tokens;
}

/** Diff display component — no context coupling, no data fetching. Hunk cursor and view mode are owned by the caller (see DiffPane). */
export function DiffRenderer({
  content,
  added,
  removed,
  loading = false,
  error,
  onRefresh,
  viewMode: viewModeProp,
  onViewModeChange,
  activeHunkId,
  onPrevHunk,
  onNextHunk,
  highlighter,
  languageForFile,
  contextLines = 0,
  onContextLinesChange,
}: DiffRendererProps) {
  const diff = useMemo(() => parseDiff(content), [content]);
  const fileRefs = useRef<(HTMLDivElement | null)[]>([]);
  const rootRef = useRef<HTMLDivElement>(null);
  const [activeFileIndex, setActiveFileIndex] = useState<number | null>(null);
  const [localViewMode, setLocalViewMode] = useState<DiffViewMode>("unified");
  // A `viewMode` prop makes the mode controlled; without one the renderer keeps its own.
  const viewMode = viewModeProp ?? localViewMode;
  const setViewMode = (mode: DiffViewMode) => {
    setLocalViewMode(mode);
    onViewModeChange?.(mode);
  };
  const tokens = useHunkTokens(diff, highlighter, languageForFile);
  const hunkIds = useMemo(() => buildHunkList(diff).map((r) => hunkDomId(r.fileIndex, r.hunkIndex)), [diff]);
  const hunkTotal = hunkIds.length;
  const activeHunkNumber = activeHunkId ? hunkIds.indexOf(activeHunkId) + 1 || null : null;

  useEffect(() => {
    if (!activeHunkId) return;
    const reduceMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
    // Scoped to this renderer: hunk ids are not unique across mounted renderers.
    rootRef.current?.querySelector(`[data-hunk-id="${activeHunkId}"]`)?.scrollIntoView?.({ block: "nearest", behavior: reduceMotion ? "auto" : "smooth" });
  }, [activeHunkId]);

  const getLineClass = (type: DiffLine["type"]) => {
    if (type === "add") return lineAdd;
    if (type === "delete") return lineDelete;
    return lineContext;
  };

  const jumpToFile = (fi: number) => {
    setActiveFileIndex(fi);
    fileRefs.current[fi]?.scrollIntoView({ behavior: "smooth", block: "start" });
  };

  const lineText = (fi: number, hi: number, li: number, l: DiffLine, withPrefix: boolean): ReactNode => {
    const prefix = withPrefix ? l.content.charAt(0) : "";
    const text = l.content.slice(1);
    const t = tokens.get(hunkKey(fi, hi));
    if (!t) return prefix + text;
    return (
      <>
        {prefix}
        {renderTokens(text, t.lineStarts[li], t.spansByLine[li])}
      </>
    );
  };

  const currentContext = contextLines > 0 ? contextLines : DEFAULT_CONTEXT_LINES;
  const fullyExpanded = contextLines === CONTEXT_WHOLE_FILE;
  const expandable = !!onContextLinesChange && !fullyExpanded;

  const hiddenAbove = (file: DiffFile, hi: number): number => {
    const h = file.changes[hi];
    if (h.oldLines === 0 && h.oldStart === 0) return 0; // new file: nothing above
    const prev = file.changes[hi - 1];
    return prev ? h.oldStart - (prev.oldStart + prev.oldLines) : h.oldStart - 1;
  };

  const expandControl = (direction: "above" | "below", count: number) => (
    <div className={expandRow}>
      <button
        type="button"
        className={expandButton}
        data-testid={`diff-expand-${direction}`}
        onClick={() => onContextLinesChange?.(currentContext + EXPAND_STEP)}
      >
        {direction === "above" ? "▲" : "▼"} Show {count} more {count === 1 ? "line" : "lines"} {direction}
      </button>
    </div>
  );

  const renderUnifiedLines = (h: DiffHunk, fi: number, hi: number) => (
    <div className={lines}>
      {h.lines.map((l, li) => (
        <div key={li} className={`${line} ${getLineClass(l.type)}`}>
          <span className={lineNumber}>{l.oldLineNumber ?? " "}</span>
          <span className={lineNumber}>{l.newLineNumber ?? " "}</span>
          <span className={lineContent}>{lineText(fi, hi, li, l, true)}</span>
        </div>
      ))}
    </div>
  );

  const renderSplitLines = (h: DiffHunk, fi: number, hi: number) => {
    return (
      <div className={splitGrid} data-testid="diff-split-grid">
        {pairSplitRows(h.lines).map((row, ri) => {
          const cell = (l: DiffLine | null, idx: number, side: "left" | "right") => {
            const tint = !l ? splitPlaceholder : l.type === "add" ? splitAdd : l.type === "delete" ? splitDelete : "";
            const num = side === "left" ? l?.oldLineNumber : l?.newLineNumber;
            return (
              <Fragment>
                <span className={`${splitNumber} ${tint} ${side === "right" ? splitRightBorder : ""}`}>{num ?? " "}</span>
                <span
                  className={`${splitContent} ${tint}`}
                  data-testid={l ? undefined : "diff-split-placeholder"}
                  data-side={side}
                >
                  {l ? lineText(fi, hi, idx, l, false) : ""}
                </span>
              </Fragment>
            );
          };
          return (
            <Fragment key={ri}>
              {cell(row.left, row.leftIdx, "left")}
              {cell(row.right, row.rightIdx, "right")}
            </Fragment>
          );
        })}
      </div>
    );
  };

  if (loading) {
    return (
      <div className={container}>
        <div className={loadingClass}>Loading diff…</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className={container}>
        <div className={errorState}>
          <p>Failed to load changes</p>
          <p className={emptyHint}>{error}</p>
          {onRefresh && (
            <button className={errorRetryButton} onClick={onRefresh}>
              Retry
            </button>
          )}
        </div>
      </div>
    );
  }

  if (diff.length === 0) {
    return (
      <div className={container}>
        <div className={emptyClass}>
          <p>No changes to display</p>
          <p className={emptyHint}>Diff will show here when there are uncommitted changes.</p>
        </div>
      </div>
    );
  }

  return (
    <div className={container} ref={rootRef}>
      <div className={toolbar}>
        <div className={stats}>
          <span className={filesChanged}>
            {diff.length} {diff.length === 1 ? "file" : "files"} changed
          </span>
          <span className={additions}>+{added}</span>
          <span className={deletions}>-{removed}</span>
        </div>
        <div className={toolbarActions}>
          {(onPrevHunk || onNextHunk) && (
            <div className={hunkNavButtons}>
              <button
                type="button"
                className={hunkNavButton}
                onClick={onPrevHunk}
                disabled={!onPrevHunk || hunkTotal === 0}
                title="Previous change (p)"
                aria-label="Previous change"
                data-testid="diff-prev-hunk"
              >
                ↑ Prev
              </button>
              <span className={hunkCounter} data-testid="diff-hunk-counter" role="status" aria-live="polite">
                {activeHunkNumber ?? "–"}/{hunkTotal}
              </span>
              <button
                type="button"
                className={hunkNavButton}
                onClick={onNextHunk}
                disabled={!onNextHunk || hunkTotal === 0}
                title="Next change (n)"
                aria-label="Next change"
                data-testid="diff-next-hunk"
              >
                ↓ Next
              </button>
            </div>
          )}
          {onContextLinesChange && (
            <button
              type="button"
              className={hunkNavButton}
              onClick={() => onContextLinesChange(fullyExpanded ? CONTEXT_SERVER_DEFAULT : CONTEXT_WHOLE_FILE)}
              title={fullyExpanded ? "Back to default context" : "Show the full file around each change"}
              data-testid="diff-expand-all"
            >
              {fullyExpanded ? "Collapse" : "Expand all"}
            </button>
          )}
          {onRefresh && (
            <button type="button" className={viewModeButton} onClick={onRefresh} title="Refresh diff" aria-label="Refresh diff">↺</button>
          )}
          <div className={viewModeToggle} role="group" aria-label="Diff view mode">
            <button
              type="button"
              className={`${viewModeButton} ${viewMode === "unified" ? viewModeButtonActive : ""}`}
              aria-pressed={viewMode === "unified"}
              onClick={() => setViewMode("unified")}
              data-testid="diff-mode-unified"
            >
              Unified
            </button>
            <button
              type="button"
              className={`${viewModeButton} ${viewMode === "split" ? viewModeButtonActive : ""}`}
              aria-pressed={viewMode === "split"}
              onClick={() => setViewMode("split")}
              title="Side-by-side (s)"
              data-testid="diff-mode-split"
            >
              Split
            </button>
          </div>
        </div>
      </div>

      <div className={body}>
        {diff.length > 1 && (
          <nav className={fileTree} aria-label="Changed files">
            {diff.map((diffFile, fi) => (
              <button
                key={fi}
                type="button"
                className={`${fileTreeItem} ${activeFileIndex === fi ? fileTreeItemActive : ""}`}
                onClick={() => jumpToFile(fi)}
                title={diffFile.filename}
              >
                <span className={fileTreeFilename}>{diffFile.filename}</span>
                <span className={fileTreeStats}>
                  <span className={additions}>+{diffFile.additions}</span>
                  <span className={deletions}>-{diffFile.deletions}</span>
                </span>
              </button>
            ))}
          </nav>
        )}

        <div className={diffContent}>
          {diff.map((diffFile, fi) => (
            <div key={fi} ref={(el) => { fileRefs.current[fi] = el; }} className={file}>
              <div className={fileHeader}>
                <span className={filename}>{diffFile.filename}</span>
                <span className={fileStats}>
                  <span className={additions}>+{diffFile.additions}</span>
                  <span className={deletions}>-{diffFile.deletions}</span>
                </span>
              </div>
              {diffFile.changes.map((h, hi) => {
                const id = hunkDomId(fi, hi);
                const above = expandable ? hiddenAbove(diffFile, hi) : 0;
                const isLast = hi === diffFile.changes.length - 1;
                const canExpandBelow = expandable && isLast && !(h.newLines === 0 && h.newStart === 0);
                return (
                  <div
                    key={hi}
                    data-hunk-id={id}
                    data-testid={id}
                    data-active={activeHunkId === id ? "true" : undefined}
                    className={`${hunk} ${syntaxScope} ${activeHunkId === id ? hunkActive : ""}`}
                  >
                    {above > 0 && expandControl("above", Math.min(above, EXPAND_STEP))}
                    <div className={hunkHeader}>
                      @@ -{h.oldStart},{h.oldLines} +{h.newStart},{h.newLines} @@
                    </div>
                    {viewMode === "split" ? renderSplitLines(h, fi, hi) : renderUnifiedLines(h, fi, hi)}
                    {canExpandBelow && expandControl("below", EXPAND_STEP)}
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
