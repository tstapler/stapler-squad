"use client";

import { useRef, useState } from "react";
import { parseDiff, type DiffLine } from "@/lib/utils/parseDiff";
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
} from "./DiffRenderer.css";

export interface DiffRendererProps {
  /** Raw unified diff string from git. */
  content: string;
  added: number;
  removed: number;
  loading?: boolean;
  /** Called when the user clicks the refresh button. */
  onRefresh?: () => void;
}

/** Pure diff display component — no context coupling, no data fetching. */
export function DiffRenderer({ content, added, removed, loading = false, onRefresh }: DiffRendererProps) {
  const diff = parseDiff(content);
  const fileRefs = useRef<(HTMLDivElement | null)[]>([]);
  const [activeFileIndex, setActiveFileIndex] = useState<number | null>(null);

  const getLineClass = (type: DiffLine["type"]) => {
    if (type === "add") return lineAdd;
    if (type === "delete") return lineDelete;
    return lineContext;
  };

  const jumpToFile = (fi: number) => {
    setActiveFileIndex(fi);
    fileRefs.current[fi]?.scrollIntoView({ behavior: "smooth", block: "start" });
  };

  if (loading) {
    return (
      <div className={container}>
        <div className={loadingClass}>Loading diff…</div>
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
    <div className={container}>
      <div className={toolbar}>
        <div className={stats}>
          <span className={filesChanged}>
            {diff.length} {diff.length === 1 ? "file" : "files"} changed
          </span>
          <span className={additions}>+{added}</span>
          <span className={deletions}>-{removed}</span>
        </div>
        <div style={{ display: "flex", gap: "0.5rem", alignItems: "center" }}>
          {onRefresh && (
            <button className={viewModeButton} onClick={onRefresh} title="Refresh diff">↺</button>
          )}
          <div className={viewModeToggle}>
            <button className={`${viewModeButton} ${viewModeButtonActive}`}>Unified</button>
            <button
              className={viewModeButton}
              disabled
              title="Split view coming soon"
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
              {diffFile.changes.map((h, hi) => (
                <div key={hi} className={hunk}>
                  <div className={hunkHeader}>
                    @@ -{h.oldStart},{h.oldLines} +{h.newStart},{h.newLines} @@
                  </div>
                  <div className={lines}>
                    {h.lines.map((l, li) => (
                      <div key={li} className={`${line} ${getLineClass(l.type)}`}>
                        <span className={lineNumber}>{l.oldLineNumber ?? " "}</span>
                        <span className={lineNumber}>{l.newLineNumber ?? " "}</span>
                        <span className={lineContent}>{l.content}</span>
                      </div>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
