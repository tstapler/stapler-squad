"use client";

import { useState, useMemo, Fragment } from "react";
import type { LogEntry } from "@/lib/hooks/useLogViewer";
import { groupLogsByPattern } from "@/lib/logs/logPatterns";
import { formatTimestampShort } from "@/lib/utils/datetime";
import { useAnalytics } from "@/lib/analytics";
import * as styles from "./PatternsView.css";

interface PatternsViewProps {
  entries: LogEntry[];
  /** Max example entries to show per expanded pattern (default: 20). */
  maxExamplesPerPattern?: number;
}

const PLACEHOLDER_RE = /(<uuid>|<path>|<obj>|<n>)/g;

const LEVEL_STYLES: Record<string, string> = {
  DEBUG: styles.levelDebug,
  INFO: styles.levelInfo,
  WARN: styles.levelWarning,
  ERROR: styles.levelError,
};

/** Renders a normalized pattern string, styling `<placeholder>` tokens distinctly from literal text. */
function PatternText({ pattern }: { pattern: string }) {
  const parts = pattern.split(PLACEHOLDER_RE);
  return (
    <>
      {parts.map((part, i) =>
        PLACEHOLDER_RE.test(part) ? (
          <span key={i} className={`${styles.placeholder}`}>
            {part}
          </span>
        ) : (
          <Fragment key={i}>{part}</Fragment>
        ),
      )}
    </>
  );
}

/**
 * PatternsView — Datadog-style "Log Patterns" clustering for the app logs
 * page: groups the currently-loaded entries by normalized message (see
 * lib/logs/logPatterns.ts, which mirrors scripts/log-group.sh's CLI
 * equivalent), most-frequent first. Expanding a row shows the raw matching
 * entries — including whatever the pattern's placeholders normalized away
 * (the actual path, UUID, or count).
 */
export function PatternsView({ entries, maxExamplesPerPattern = 20 }: PatternsViewProps) {
  const { track } = useAnalytics();
  const [expandedKey, setExpandedKey] = useState<string | null>(null);
  const groups = useMemo(() => groupLogsByPattern(entries), [entries]);

  if (groups.length === 0) {
    return (
      <div className={`${styles.container}`}>
        <div className={`${styles.empty}`}>No log entries loaded to cluster into patterns.</div>
      </div>
    );
  }

  return (
    <div className={`${styles.container}`} data-testid="patterns-view">
      {groups.map((group) => {
        const isOpen = expandedKey === group.key;
        return (
          <div key={group.key} className={`${styles.row}`}>
            <div
              className={`${styles.rowHeader}`}
              role="button"
              tabIndex={0}
              aria-expanded={isOpen}
              onClick={() => {
                track({ name: "log_pattern_toggled", category: "user_action", labels: { expanded: String(!isOpen) } });
                setExpandedKey(isOpen ? null : group.key);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  track({ name: "log_pattern_toggled", category: "user_action", labels: { expanded: String(!isOpen) } });
                  setExpandedKey(isOpen ? null : group.key);
                }
              }}
            >
              <span className={`${styles.disclosure} ${isOpen ? styles.disclosureOpen : ""}`}>▶</span>
              <span className={`${styles.count}`}>{group.count.toLocaleString()}</span>
              <span className={`${styles.level} ${LEVEL_STYLES[group.level] ?? ""}`}>{group.level}</span>
              <span className={`${styles.pattern}`}>
                <PatternText pattern={group.pattern} />
              </span>
            </div>
            {isOpen && (
              <div className={`${styles.examples}`}>
                {group.entries.slice(0, maxExamplesPerPattern).map((entry) => (
                  <div key={entry.id} className={`${styles.example}`}>
                    <span className={`${styles.exampleTimestamp}`}>
                      {formatTimestampShort(new Date(entry.timestamp))}
                    </span>
                    <span className={`${styles.exampleMessage}`}>{entry.message}</span>
                  </div>
                ))}
                {group.entries.length > maxExamplesPerPattern && (
                  <div className={`${styles.examplesMore}`}>
                    …and {(group.entries.length - maxExamplesPerPattern).toLocaleString()} more
                  </div>
                )}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}

export default PatternsView;
