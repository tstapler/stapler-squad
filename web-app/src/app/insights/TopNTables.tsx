// +feature: insights-dashboard
"use client";

import type { TopEntry } from "@/gen/session/v1/insights_pb";
import { tableCard, tableTitle, table, th, thRight, td, tdRight, empty } from "./TopNTables.css";
import { fmtTokens } from "./insightsFormatters";

interface TopTableProps {
  title: string;
  entries: TopEntry[];
  valueLabel?: string;
}

export function TopNTable({ title: tableHeading, entries, valueLabel = "Tokens" }: TopTableProps) {
  return (
    <div className={tableCard}>
      <div className={tableTitle}>{tableHeading}</div>
      {entries.length === 0 ? (
        <div className={empty}>No data</div>
      ) : (
        <table className={table}>
          <thead>
            <tr>
              <th className={th}>Name</th>
              <th className={thRight}>{valueLabel}</th>
              <th className={thRight}>Uses</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e, i) => (
              <tr key={`${e.name}-${i}`}>
                <td className={td} title={e.name}>{e.name || "—"}</td>
                <td className={tdRight}>{fmtTokens(e.tokenCount)}</td>
                <td className={tdRight}>{e.activationCount}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
