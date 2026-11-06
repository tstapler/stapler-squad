"use client";

import * as styles from "./ResultPanel.css";

/** "View raw JSON" toggle shared by the formatted result panels. */
export function RawJsonDisclosure({ value, testId }: { value: unknown; testId: string }) {
  return (
    <details className={styles.disclosure} data-testid={testId}>
      <summary className={styles.disclosureSummary}>View raw JSON</summary>
      <pre className={styles.rawJson}>{JSON.stringify(value, null, 2)}</pre>
    </details>
  );
}
