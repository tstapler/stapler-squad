// +feature: escape-analytics

import * as styles from "./MangleRateIndicator.css";

interface MangleRateIndicatorProps {
  mangleRate: number;
  totalSequences: bigint;
  totalMangled: bigint;
}

type Severity = "good" | "warning" | "error";

function getSeverity(mangleRate: number): Severity {
  if (mangleRate < 0.01) return "good";
  if (mangleRate <= 0.05) return "warning";
  return "error";
}

function getSeverityLabel(severity: Severity): string {
  switch (severity) {
    case "good": return "Healthy";
    case "warning": return "Elevated";
    case "error": return "High";
  }
}

export function MangleRateIndicator({
  mangleRate,
  totalSequences,
  totalMangled,
}: MangleRateIndicatorProps) {
  const severity = getSeverity(mangleRate);
  const percentage = (mangleRate * 100).toFixed(2);

  return (
    <div className={styles.container} data-testid="mangle-rate-indicator">
      <div className={styles.rateRow}>
        <span
          className={styles.rateValue({ severity })}
          aria-label={`Mangle rate: ${percentage} percent`}
          data-testid="mangle-rate-value"
        >
          {percentage}%
        </span>
        <span
          className={styles.rateBadge({ severity })}
          data-testid="mangle-rate-badge"
          data-severity={severity}
        >
          {getSeverityLabel(severity)}
        </span>
      </div>

      <p className={styles.subtitle}>
        Mangle rate — proportion of escape sequences that were altered during processing
      </p>

      <div className={styles.countsRow} data-testid="mangle-counts">
        <div className={styles.countItem}>
          <span className={styles.countValue}>{totalSequences.toString()}</span>
          <span className={styles.countLabel}>Total sequences</span>
        </div>
        <div className={styles.countItem}>
          <span className={styles.countValue}>{totalMangled.toString()}</span>
          <span className={styles.countLabel}>Mangled</span>
        </div>
      </div>
    </div>
  );
}
