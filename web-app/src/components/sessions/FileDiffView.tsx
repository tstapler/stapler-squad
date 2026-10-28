"use client";

import { DiffPane } from "./DiffPane";
import * as styles from "./FileDiffView.css";

interface FileDiffViewProps {
  path: string;
  /** Raw unified diff for just this file. */
  content: string;
  added: number;
  removed: number;
  onBack: () => void;
  /** Open the file itself (Files tab). */
  onOpenFile?: (path: string) => void;
  contextLines?: number;
  onContextLinesChange?: (contextLines: number) => void;
}

/** Single-file diff shown in place of the VCS file list, with a way back. */
export function FileDiffView({
  path,
  content,
  added,
  removed,
  onBack,
  onOpenFile,
  contextLines,
  onContextLinesChange,
}: FileDiffViewProps) {
  return (
    <div className={styles.wrapper} data-testid="file-diff-view">
      <div className={styles.bar}>
        <button type="button" className={styles.backButton} onClick={onBack} data-testid="file-diff-back">
          ← Back to file list
        </button>
        <span className={styles.path} title={path}>
          {path}
        </span>
        {onOpenFile && (
          <button type="button" className={styles.backButton} onClick={() => onOpenFile(path)}>
            Open file
          </button>
        )}
      </div>
      <div className={styles.diff}>
        <DiffPane
          autoFocus
          content={content}
          added={added}
          removed={removed}
          contextLines={contextLines}
          onContextLinesChange={onContextLinesChange}
        />
      </div>
    </div>
  );
}
