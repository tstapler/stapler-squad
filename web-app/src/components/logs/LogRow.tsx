import React from "react";
import type { LogEntry } from "@/lib/hooks/useLogViewer";
import { segmentText } from "@/lib/logs/logParser";
import {
  levelBadge,
  rowTint,
  gutterAbsolute,
  bodyScrollable,
  lineNumber,
  timestampFull,
  timestampShort,
  mark,
} from "./LogRow.css";

interface LogRowProps {
  entry: LogEntry;
  index: number;
  isExpanded: boolean;
  isSelected: boolean;
  onToggle: () => void;
  searchQuery: string;
}

/**
 * Extract HH:mm:ss from an ISO-like timestamp string.
 * Returns the original string unchanged if parsing fails.
 */
function abbreviateTimestamp(ts: string): string {
  // Handles "2026-01-02T15:04:05.000Z", "2026-01-02 15:04:05", etc.
  const match = ts.match(/T?(\d{2}:\d{2}:\d{2})/);
  return match ? match[1] : ts;
}

export const LogRow = React.memo(function LogRow({
  entry,
  index,
  isExpanded,
  isSelected,
  onToggle,
  searchQuery,
}: LogRowProps) {
  const level = entry.level;
  const segments = segmentText(entry.message, searchQuery);
  const shortTs = abbreviateTimestamp(entry.timestamp);

  return (
    <div
      className={rowTint({ level, isSelected })}
      role="row"
      aria-expanded={isExpanded}
      aria-selected={isSelected}
      tabIndex={isSelected ? 0 : -1}
      data-testid={`log-row-${index}`}
      onClick={onToggle}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onToggle();
        }
      }}
    >
      {/*
        T1: Absolutely-positioned gutter — always visible at left:0 regardless
        of how far the bodyScrollable div is scrolled horizontally. This is the
        "overlay" variant of split-column layout that fixes position:sticky
        inside overflow-x:auto on iOS Safari ≤ 16.
      */}
      <div className={gutterAbsolute}>
        <span className={lineNumber}>{index + 1}</span>
        <span className={levelBadge({ level })}>{level === "UNKNOWN" ? "" : level}</span>
      </div>

      {/*
        T1: Scrollable body with padding-left matching gutterAbsolute width so
        text begins after the gutter overlay. overflow-x:auto here, not on the
        outer row, so the gutter stays visible during horizontal scroll.
        T4: Two timestamp spans — full shown on wide, short shown on narrow —
        toggled by CSS @media without any JS.
      */}
      <div className={bodyScrollable}>
        <span className={timestampFull}>{entry.timestamp}</span>
        <span className={timestampShort}>{shortTs}</span>
        {segments.map((seg, i) =>
          seg.highlight ? (
            <mark key={i} className={mark}>
              {seg.text}
            </mark>
          ) : (
            <span key={i}>{seg.text}</span>
          ),
        )}
      </div>
    </div>
  );
});
