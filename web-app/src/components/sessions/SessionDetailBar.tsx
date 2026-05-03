"use client";

import { Kbd } from "@/components/ui/Kbd";
import {
  bar,
  branchName,
  pathText,
  shortcutHints,
  hint,
  backButton,
} from "./SessionDetailBar.css";

interface SessionDetailBarProps {
  branch?: string;
  path?: string;
  statusBadge?: React.ReactNode;
  onBack?: () => void;
}

/**
 * SessionDetailBar — compact single-row bar shown above the terminal.
 * Shows branch name, status badge, path, and keyboard shortcut hints.
 * On mobile (<768px) shows a back button instead of shortcut hints.
 */
export function SessionDetailBar({
  branch,
  path,
  statusBadge,
  onBack,
}: SessionDetailBarProps) {
  return (
    <div className={bar}>
      {onBack && (
        <button className={backButton} onClick={onBack} aria-label="Back to session list">
          ← Back
        </button>
      )}
      {branch && <span className={branchName}>{branch}</span>}
      {statusBadge}
      {path && <span className={pathText}>{path}</span>}
      <div className={shortcutHints}>
        <span className={hint}>
          <Kbd size="sm">t</Kbd> terminal
        </span>
        <span className={hint}>
          <Kbd size="sm">p</Kbd> pause
        </span>
        <span className={hint}>
          <Kbd size="sm">r</Kbd> resume
        </span>
      </div>
    </div>
  );
}
