// +feature: escape-analytics

import type { EscapeSequenceCount } from "@/gen/session/v1/session_pb";
import * as styles from "./SequenceHistogram.css";

interface SequenceHistogramProps {
  histogram: EscapeSequenceCount[];
}

export function SequenceHistogram({ histogram }: SequenceHistogramProps) {
  if (histogram.length === 0) {
    return (
      <div className={styles.container}>
        <p className={styles.emptyState}>No escape sequences recorded.</p>
      </div>
    );
  }

  // Find the max count to compute relative bar widths
  const maxCount = histogram.reduce(
    (max, entry) => (entry.count > max ? entry.count : max),
    0n
  );

  return (
    <div className={styles.container} role="list" aria-label="Escape sequence histogram">
      {histogram.map((entry) => {
        const totalWidth = maxCount > 0n
          ? Number((entry.count * 100n) / maxCount)
          : 0;
        const mangledWidth = entry.count > 0n
          ? Number((entry.mangledCount * 100n) / entry.count)
          : 0;
        const cleanWidth = totalWidth - (totalWidth * mangledWidth) / 100;
        const mangledFillWidth = (totalWidth * mangledWidth) / 100;

        return (
          <div
            key={entry.sequenceType}
            className={styles.row}
            role="listitem"
            aria-label={`${entry.sequenceType}: ${entry.count} total, ${entry.mangledCount} mangled`}
          >
            <span className={styles.label} title={entry.sequenceType}>
              {entry.sequenceType || "(unknown)"}
            </span>
            <div className={styles.barTrack}>
              <div
                className={styles.barFill}
                style={{ width: `${cleanWidth}%` }}
                aria-hidden="true"
              />
              {entry.mangledCount > 0n && (
                <div
                  className={styles.barMangledFill}
                  style={{ width: `${mangledFillWidth}%` }}
                  aria-hidden="true"
                />
              )}
            </div>
            <span className={styles.countLabel}>
              {entry.count.toString()}
              {entry.mangledCount > 0n && (
                <> / <span style={{ color: "var(--error-inline)" }}>{entry.mangledCount.toString()}</span></>
              )}
            </span>
          </div>
        );
      })}

      <div className={styles.legend}>
        <div className={styles.legendItem}>
          <span className={styles.legendDot} aria-hidden="true" />
          <span>Total</span>
        </div>
        <div className={styles.legendItem}>
          <span className={styles.legendDotMangled} aria-hidden="true" />
          <span>Mangled</span>
        </div>
      </div>
    </div>
  );
}
