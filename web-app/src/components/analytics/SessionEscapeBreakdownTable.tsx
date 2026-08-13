// +feature: escape-analytics

import { useMemo, useState } from "react";
import type { SessionEscapeSummary } from "@/gen/session/v1/session_pb";
import { MangleRateIndicator } from "./MangleRateIndicator";
import { srOnly } from "@/components/ui/LiveRegion.css";
import * as styles from "./SessionEscapeBreakdownTable.css";

interface SessionEscapeBreakdownTableProps {
  rows: SessionEscapeSummary[];
  /** Fleet-wide mangle rate, used to compute the outlier threshold for each row. */
  fleetMangleRate: number;
  loading?: boolean;
}

type SortColumn = "sessionId" | "totalSequences" | "totalMangled" | "mangleRate";
type SortDirection = "ascending" | "descending";

interface ColumnDef {
  key: SortColumn;
  label: string;
}

const COLUMNS: ColumnDef[] = [
  { key: "sessionId", label: "Session" },
  { key: "totalSequences", label: "Total Sequences" },
  { key: "totalMangled", label: "Total Mangled" },
  { key: "mangleRate", label: "Mangle Rate" },
];

// Outlier threshold per ux.md §3: a session is an outlier when its mangle
// rate exceeds 2x the fleet-wide rate, or a flat 5% floor when the
// fleet-wide rate is ~0 (2x of ~0 would otherwise flag nearly every row).
const OUTLIER_FLOOR = 0.05;
const FLEET_RATE_EPSILON = 0.001;

function isOutlier(rowMangleRate: number, fleetMangleRate: number): boolean {
  const threshold =
    fleetMangleRate > FLEET_RATE_EPSILON ? fleetMangleRate * 2 : OUTLIER_FLOOR;
  return rowMangleRate > threshold;
}

function compareRows(
  a: SessionEscapeSummary,
  b: SessionEscapeSummary,
  column: SortColumn
): number {
  switch (column) {
    case "sessionId":
      return a.sessionId.localeCompare(b.sessionId);
    case "totalSequences":
      return a.totalSequences === b.totalSequences
        ? 0
        : a.totalSequences > b.totalSequences
          ? 1
          : -1;
    case "totalMangled":
      return a.totalMangled === b.totalMangled
        ? 0
        : a.totalMangled > b.totalMangled
          ? 1
          : -1;
    case "mangleRate":
      return a.mangleRate - b.mangleRate;
  }
}

export function SessionEscapeBreakdownTable({
  rows,
  fleetMangleRate,
  loading = false,
}: SessionEscapeBreakdownTableProps) {
  // Default sort: mangleRate descending (ux.md §3 / plan.md Task 2.4.4).
  const [sortColumn, setSortColumn] = useState<SortColumn>("mangleRate");
  const [sortDirection, setSortDirection] = useState<SortDirection>("descending");

  const sortedRows = useMemo(() => {
    const sorted = [...rows].sort((a, b) => compareRows(a, b, sortColumn));
    if (sortDirection === "descending") sorted.reverse();
    return sorted;
  }, [rows, sortColumn, sortDirection]);

  const handleSort = (column: SortColumn) => {
    if (column === sortColumn) {
      setSortDirection((prev) => (prev === "ascending" ? "descending" : "ascending"));
    } else {
      setSortColumn(column);
      setSortDirection("descending");
    }
  };

  if (!loading && rows.length === 0) {
    return (
      <div className={styles.container}>
        <p className={styles.emptyState}>No per-session breakdown available.</p>
      </div>
    );
  }

  return (
    <div className={styles.container}>
      <table className={styles.table} data-testid="session-escape-breakdown-table">
        <thead className={styles.thead}>
          <tr>
            {COLUMNS.map((column) => {
              const active = column.key === sortColumn;
              const ariaSort = active ? sortDirection : "none";
              return (
                <th key={column.key} className={styles.th} scope="col" aria-sort={ariaSort}>
                  <button
                    type="button"
                    className={styles.sortButton}
                    onClick={() => handleSort(column.key)}
                    data-testid={`sort-button-${column.key}`}
                  >
                    {column.label}
                    {active && (
                      <span className={styles.sortIcon} aria-hidden="true">
                        {sortDirection === "ascending" ? "▲" : "▼"}
                      </span>
                    )}
                  </button>
                </th>
              );
            })}
          </tr>
        </thead>
        <tbody>
          {sortedRows.map((row) => {
            const outlier = isOutlier(row.mangleRate, fleetMangleRate);
            return (
              <tr
                key={row.sessionId}
                className={outlier ? styles.outlierRow : styles.tr}
                data-testid="session-escape-breakdown-row"
                data-outlier={outlier ? "true" : "false"}
              >
                <td className={styles.td}>
                  {outlier && (
                    <>
                      <span className={styles.outlierIcon} aria-hidden="true">
                        ⚠
                      </span>
                      <span className={srOnly}>Outlier: </span>
                    </>
                  )}
                  {row.sessionId}
                </td>
                <td className={styles.td}>{row.totalSequences.toString()}</td>
                <td className={styles.td}>{row.totalMangled.toString()}</td>
                <td className={styles.td}>
                  <MangleRateIndicator
                    mangleRate={row.mangleRate}
                    totalSequences={row.totalSequences}
                    totalMangled={row.totalMangled}
                  />
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>

      {loading && (
        <p className={styles.loadingState} role="status" aria-live="polite">
          Loading…
        </p>
      )}
    </div>
  );
}
