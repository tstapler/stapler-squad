"use client";

// +feature: backlog-triage-results-panel

import type { TriageResult } from "@/lib/hooks/useBacklogService";
import { useFeatureFlag } from "@/lib/contexts/FeatureFlagsContext";
import { RawJsonDisclosure } from "./RawJsonDisclosure";
import * as styles from "./ResultPanel.css";

export const TRIAGE_RESULTS_PANEL_FLAG = "backlog-triage-results-panel";

export interface TriageResultPanelProps {
  /** Undefined hides the panel (no triage has run); `loading` shows a skeleton instead. */
  triageResult: TriageResult | undefined;
  /** Item priority (1-5) — TriageResult itself carries none. */
  priority?: number;
  loading?: boolean;
}

function isMalformed(r: TriageResult): boolean {
  return typeof r.summary !== "string" || !Array.isArray(r.suggestions ?? []) || !Array.isArray(r.tasks ?? []);
}

export function TriageResultPanel({ triageResult, priority, loading }: TriageResultPanelProps) {
  const enabled = useFeatureFlag(TRIAGE_RESULTS_PANEL_FLAG);
  if (!enabled) return null;

  if (loading) {
    return (
      <section className={styles.container} aria-busy="true" aria-label="Triage Results" data-testid="triage-result-skeleton">
        <div className={styles.skeletonLine} style={{ width: "40%" }} />
        <div className={styles.skeletonLine} />
        <div className={styles.skeletonLine} style={{ width: "70%" }} />
      </section>
    );
  }
  if (!triageResult) return null;

  if (isMalformed(triageResult)) {
    return (
      <section className={styles.container} aria-label="Triage Results" data-testid="triage-result-panel">
        <h3 className={styles.title}>Triage Results</h3>
        <p className={styles.mutedText}>Triage result could not be displayed.</p>
        <RawJsonDisclosure value={triageResult} testId="triage-raw-json" />
      </section>
    );
  }

  const tasks = triageResult.tasks ?? [];
  const suggestions = (triageResult.suggestions ?? []).filter((s) => s.rationale !== "question");

  return (
    <section className={styles.container} aria-label="Triage Results" data-testid="triage-result-panel">
      <div className={styles.header}>
        <h3 className={styles.title}>Triage Results</h3>
        {priority != null && <span className={styles.badge}>P{priority}</span>}
      </div>
      {triageResult.summary && <p>{triageResult.summary}</p>}
      {tasks.length > 0 && (
        <>
          <h4 className={styles.sectionLabel}>Tasks ({tasks.length})</h4>
          <ul className={styles.list}>
            {tasks.map((t, i) => (
              <li key={i} className={styles.row}>
                <span className={styles.rowText}>{t.text}</span>
                {t.estimate && <span className={styles.badge}>{t.estimate}</span>}
                {t.category && <span className={styles.badge}>{t.category}</span>}
              </li>
            ))}
          </ul>
        </>
      )}
      {suggestions.length > 0 && (
        <>
          <h4 className={styles.sectionLabel}>Suggestions ({suggestions.length})</h4>
          <ul className={styles.list}>
            {suggestions.map((s, i) => (
              <li key={i} className={styles.row}>
                <span className={styles.rowText}>{s.text}</span>
                {s.rationale && <span className={styles.evidence}>{s.rationale}</span>}
              </li>
            ))}
          </ul>
        </>
      )}
      <RawJsonDisclosure value={triageResult} testId="triage-raw-json" />
    </section>
  );
}
