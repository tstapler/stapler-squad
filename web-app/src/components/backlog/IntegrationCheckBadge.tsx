"use client";
// +feature: backlog:integration-check-badge

import * as styles from "./IntegrationCheckBadge.css";

/** Marks an epic's auto-created end-to-end check child in backlog lists. */
export function IntegrationCheckBadge() {
  return (
    <span className={styles.badge} data-testid="integration-check-badge">
      Integration Check
    </span>
  );
}
