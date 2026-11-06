"use client";

import * as styles from "./ResultPanel.css";

const DIAGNOSIS_PREFIX = "Diagnosis result: ";

/** Splits a diagnose activity note into outcome + summary; null if it isn't one. */
export function parseDiagnosisNote(message: string): { outcome: string; summary: string } | null {
  if (!message.startsWith(DIAGNOSIS_PREFIX)) return null;
  const rest = message.slice(DIAGNOSIS_PREFIX.length);
  const sep = rest.indexOf("\n\n");
  const outcome = (sep === -1 ? rest : rest.slice(0, sep)).trim();
  if (!/^[A-Z_]+$/.test(outcome)) return null;
  return { outcome, summary: sep === -1 ? "" : rest.slice(sep + 2).trim() };
}

export function DiagnoseResultCard({ message }: { message: string }) {
  const parsed = parseDiagnosisNote(message);
  // Parse-error fallback: unrecognized shape renders as the original plain text.
  if (!parsed) return <span>{message}</span>;
  const badge = parsed.outcome === "BUG_FILED" ? styles.badgeFail : parsed.outcome === "NO_ACTION" ? styles.badge : styles.badgeWarn;
  return (
    <div className={styles.container} data-testid="diagnose-result-card">
      <div className={styles.header}>
        <h4 className={styles.title}>Diagnosis</h4>
        <span className={badge}>{parsed.outcome}</span>
      </div>
      {parsed.summary && <p className={styles.mutedText}>{parsed.summary}</p>}
    </div>
  );
}
