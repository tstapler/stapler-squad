"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { parseDiff } from "@/lib/utils/parseDiff";
import { buildHunkList, hunkDomId, stepHunkIndex } from "@/lib/diff/diffNavigation";
import { useShortcut } from "@/lib/shortcuts/useShortcut";
import { DiffRenderer, type DiffViewMode } from "@/components/shared/DiffRenderer";
import { LezerHighlighter } from "./files/engine/LezerHighlighter";
import { detectLanguage } from "./FileContentViewer";
import { pane } from "./DiffPane.css";

interface DiffPaneProps {
  content: string;
  added: number;
  removed: number;
  loading?: boolean;
  onRefresh?: () => void;
  contextLines?: number;
  onContextLinesChange?: (contextLines: number) => void;
  header?: ReactNode;
}

/**
 * DiffRenderer plus the stateful behaviour that makes it navigable: hunk
 * cursor, n/p/s shortcuts (scoped to the "diff" shortcut context by this pane
 * holding focus), split/unified mode and Lezer syntax highlighting.
 */
export function DiffPane({
  content,
  added,
  removed,
  loading,
  onRefresh,
  contextLines,
  onContextLinesChange,
  header,
}: DiffPaneProps) {
  const paneRef = useRef<HTMLDivElement>(null);
  const highlighter = useMemo(() => new LezerHighlighter(), []);
  const [viewMode, setViewMode] = useState<DiffViewMode>("unified");
  const [activeHunk, setActiveHunk] = useState<number | null>(null);

  const hunks = useMemo(() => buildHunkList(parseDiff(content)), [content]);

  // The diff changed underneath the cursor (refresh, context change): drop a now-invalid index.
  useEffect(() => {
    setActiveHunk((cur) => (cur !== null && cur >= hunks.length ? null : cur));
  }, [hunks.length]);

  // n/p only dispatch while focus is inside a [data-context="diff"] element.
  useEffect(() => {
    paneRef.current?.focus({ preventScroll: true });
  }, []);

  const navigate = useCallback(
    (direction: "next" | "prev") => setActiveHunk((cur) => stepHunkIndex(cur, hunks.length, direction)),
    [hunks.length],
  );
  const next = useCallback(() => navigate("next"), [navigate]);
  const prev = useCallback(() => navigate("prev"), [navigate]);
  const toggleMode = useCallback(() => setViewMode((m) => (m === "unified" ? "split" : "unified")), []);

  useShortcut("diff.hunk.next", { key: "n", context: "diff", label: "Next change", action: next });
  useShortcut("diff.hunk.prev", { key: "p", context: "diff", label: "Previous change", action: prev });
  useShortcut("diff.view.toggle", { key: "s", context: "diff", label: "Toggle split / unified view", action: toggleMode });

  const activeRef = activeHunk === null ? undefined : hunks[activeHunk];
  const activeHunkId = activeRef ? hunkDomId(activeRef.fileIndex, activeRef.hunkIndex) : null;

  return (
    <div ref={paneRef} className={pane} data-context="diff" tabIndex={0} data-testid="diff-pane">
      {header}
      <DiffRenderer
        content={content}
        added={added}
        removed={removed}
        loading={loading}
        onRefresh={onRefresh}
        viewMode={viewMode}
        onViewModeChange={setViewMode}
        activeHunkId={activeHunkId}
        onPrevHunk={prev}
        onNextHunk={next}
        highlighter={highlighter}
        languageForFile={detectLanguage}
        contextLines={contextLines}
        onContextLinesChange={onContextLinesChange}
      />
    </div>
  );
}
