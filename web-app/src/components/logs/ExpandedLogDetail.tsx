"use client";

import { tryParseJson } from "@/lib/logs/logParser";
import type { LogEntry } from "@/lib/hooks/useLogViewer";
import * as styles from "./ExpandedLogDetail.css";

interface ExpandedLogDetailProps {
  entry: LogEntry;
}

/**
 * ExpandedLogDetail — accordion detail panel for a log entry.
 * Epic 4: JSON pretty-print detection and one-tap copy (with mobile fallback).
 */
export function ExpandedLogDetail({ entry }: ExpandedLogDetailProps) {
  const text = entry.raw || entry.message;
  const parsed = tryParseJson(text);

  const handleCopy = () => {
    navigator.clipboard.writeText(text).catch(() => {
      // Fallback for browsers without clipboard API (some mobile WebViews)
      const el = document.createElement("textarea");
      el.value = text;
      document.body.appendChild(el);
      el.select();
      document.execCommand("copy");
      document.body.removeChild(el);
    });
  };

  return (
    <div className={styles.detailPanel} role="cell" aria-label="Log entry detail">
      <div className={styles.detailHeader}>
        <span className={styles.detailLabel}>Full entry</span>
        <button
          className={styles.copyButton}
          onClick={handleCopy}
          aria-label="Copy log entry"
          type="button"
        >
          Copy
        </button>
      </div>
      {parsed ? (
        <pre className={styles.jsonBlock}>{JSON.stringify(parsed, null, 2)}</pre>
      ) : (
        <pre className={styles.rawBlock}>{text}</pre>
      )}
    </div>
  );
}
