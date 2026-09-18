// +feature: insights-dashboard
"use client";

import type { TopEntry } from "@/gen/session/v1/insights_pb";
import { tableCard, tableTitle, table, th, thRight, td, tdRight, empty } from "./TopNTables.css";

interface TopTableProps {
  title: string;
  entries: TopEntry[];
}

// Only activationCount (uses/activations) is populated by the backend today —
// per-tool/per-skill token attribution doesn't exist in the token pipeline yet
// (see session/tokens: SkillActivation and ToolTokenStats carry no token
// fields), so a "Tokens" column would always render zero. Show Uses only
// until that attribution is built.
export function TopNTable({ title: tableHeading, entries }: TopTableProps) {
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
              <th className={thRight}>Uses</th>
            </tr>
          </thead>
          <tbody>
            {entries.map((e, i) => (
              <tr key={`${e.name}-${i}`}>
                <td className={td} title={e.name}>{e.name || "—"}</td>
                <td className={tdRight}>{e.activationCount}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
