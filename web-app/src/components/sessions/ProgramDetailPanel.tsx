// +feature: analytics-drill-down
"use client";

import React, { useMemo } from "react";
import { useProgramAnalytics } from "@/lib/hooks/useProgramAnalytics";
import type { SubcommandBreakdownProto } from "@/gen/session/v1/types_pb";
import * as styles from "./ProgramDetailPanel.css";

interface ProgramDetailPanelProps {
  program: string;
  windowDays: number;
  onClose: () => void;
}

export function ProgramDetailPanel({
  program,
  windowDays,
  onClose,
}: ProgramDetailPanelProps) {
  const { data, isLoading, error } = useProgramAnalytics(program, windowDays);

  const maxTotal = useMemo(() => {
    if (!data?.subcommands?.length) return 1;
    return Math.max(...data.subcommands.map((s) => s.total), 1);
  }, [data?.subcommands]);

  const totalForProgram = useMemo(() => {
    if (!data?.subcommands?.length) return 0;
    return data.subcommands.reduce((sum, s) => sum + s.total, 0);
  }, [data?.subcommands]);

  return (
    <div className={styles.panel} data-testid="program-detail-panel">
      <div className={styles.panelHeader}>
        <span className={styles.panelTitle}>
          {program}
          {data?.category ? ` · ${data.category}` : ""}
        </span>
        <button
          className={styles.closeButton}
          onClick={onClose}
          aria-label="Close program detail panel"
        >
          ✕
        </button>
      </div>

      {isLoading && <div className={styles.loadingState}>Loading…</div>}

      {error && (
        <div className={styles.errorState} role="alert">
          Failed to load analytics: {error.message}
        </div>
      )}

      {data && !isLoading && (
        <>
          {/* Subcommand frequency table — AC-9 */}
          <div className={styles.sectionTitle}>Subcommand Breakdown</div>
          <table className={styles.breakdownTable}>
            <thead>
              <tr>
                <th className={styles.th}>Subcommand</th>
                <th className={styles.th}>Count</th>
                <th className={styles.th}>%</th>
                <th className={styles.th}>Allow</th>
                <th className={styles.th}>Deny</th>
                <th className={styles.th}>Manual</th>
                <th className={styles.th}>Coverage</th>
                <th className={styles.th}></th>
              </tr>
            </thead>
            <tbody>
              {data.subcommands.map((row) => (
                <SubcommandRow
                  key={row.subcommand || "(none)"}
                  row={row}
                  program={program}
                  totalForProgram={totalForProgram}
                  maxTotal={maxTotal}
                />
              ))}
              {data.subcommands.length === 0 && (
                <tr>
                  <td className={styles.td} colSpan={8}>
                    No subcommand data for this program in the selected window.
                  </td>
                </tr>
              )}
            </tbody>
          </table>

          {/* Example commands — AC-10 */}
          {data.recentExamples.length > 0 && (
            <>
              <div className={styles.sectionTitle}>Recent Examples</div>
              <ul className={styles.examplesList}>
                {data.recentExamples.map((cmd, i) => (
                  <li key={i} className={styles.exampleItem}>
                    {cmd}
                  </li>
                ))}
              </ul>
            </>
          )}
        </>
      )}
    </div>
  );
}

interface SubcommandRowProps {
  row: SubcommandBreakdownProto;
  program: string;
  totalForProgram: number;
  maxTotal: number;
}

function SubcommandRow({
  row,
  program,
  totalForProgram,
  maxTotal,
}: SubcommandRowProps) {
  const label = row.subcommand || "(none)";
  const pct =
    totalForProgram > 0
      ? ((row.total / totalForProgram) * 100).toFixed(1)
      : "0.0";
  const barWidth = maxTotal > 0 ? Math.round((row.total / maxTotal) * 80) : 0;
  const manual = row.escalate + row.manualAllow + row.manualDeny;

  // AC-13: "Add rule →" link pre-populates the rule form
  const addRuleHref = `/rules?program=${encodeURIComponent(program)}&subcommand=${encodeURIComponent(row.subcommand)}`;

  return (
    <tr>
      <td className={styles.td}>{label}</td>
      <td className={styles.td}>
        <span
          className={styles.sparklineBar}
          style={{ width: `${barWidth}px` }}
          aria-hidden="true"
        />
        {" "}
        {row.total}
      </td>
      <td className={styles.td}>{pct}%</td>
      <td className={styles.td}>{row.autoAllow}</td>
      <td className={styles.td}>{row.autoDeny}</td>
      <td className={styles.td}>{manual}</td>
      <td className={styles.td}>
        {row.hasRuleCoverage ? (
          <span className={styles.coverageYes}>✓ covered</span>
        ) : (
          <span className={styles.coverageNo}>✗ gap</span>
        )}
      </td>
      <td className={styles.td}>
        {!row.hasRuleCoverage && (
          <a href={addRuleHref} className={styles.addRuleLink}>
            Add rule →
          </a>
        )}
      </td>
    </tr>
  );
}
