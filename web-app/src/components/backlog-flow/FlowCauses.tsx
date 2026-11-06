"use client";

import { useState } from "react";
import type { FlowCauseRow, FlowCausesSection, FlowTrendsSection } from "@/gen/session/v1/backlog_flow_pb";
import { DrillButton } from "./DrillButton";
import { CauseSparkline } from "./FlowTrends";
import { formatInt, plural } from "./flowFormatters";
import * as styles from "./flow.css";

interface FlowCausesProps {
  causes: FlowCausesSection;
  /** Estimated codes of the causes section; `pr_fix_cause:unavailable` changes the PR fix label. */
  estimated: readonly string[];
  /** Weekly rework by cause; each row embeds its own sparkline when present. */
  trends?: FlowTrendsSection;
  onDrill: (key: string) => void;
}

type SortKey = "cause" | "count";
type SortDir = "ascending" | "descending";
interface Sort {
  key: SortKey;
  dir: SortDir;
}

const PR_FIX_UNAVAILABLE = "pr_fix_cause:unavailable";
const DEFAULT_SORT: Sort = { key: "count", dir: "descending" };
const FIRST_DIRECTION: Record<SortKey, SortDir> = { cause: "ascending", count: "descending" };
const CAUSE_LABELS: Record<string, string> = {
  review_fail: "Review FAIL rework",
  pr_fix: "PR fix",
  bounce: "Bounce",
  other: "Other",
};

function causeLabel(cause: string, prFixSeparable: boolean): string {
  if (cause === "pr_fix" && !prFixSeparable) return "PR fix (CI/review/conflict not separable until trigger capture)";
  return CAUSE_LABELS[cause] ?? cause;
}

function sortRows(rows: readonly FlowCauseRow[], sort: Sort, label: (cause: string) => string): FlowCauseRow[] {
  const sign = sort.dir === "ascending" ? 1 : -1;
  return [...rows].sort((a, b) =>
    sort.key === "count" ? sign * ((a.count?.value ?? 0) - (b.count?.value ?? 0)) : sign * label(a.cause).localeCompare(label(b.cause)),
  );
}

interface SortHeaderProps {
  sortKey: SortKey;
  sort: Sort;
  onSort: (key: SortKey) => void;
  children: string;
}

function SortHeader({ sortKey, sort, onSort, children }: SortHeaderProps) {
  const active = sort.key === sortKey;
  return (
    <th scope="col" className={styles.cell} aria-sort={active ? sort.dir : "none"}>
      <button type="button" className={styles.sortButton} onClick={() => onSort(sortKey)}>
        {children}
      </button>
    </th>
  );
}

/** Ranked rework causes; each count is a drill button and each header sorts. */
export function FlowCauses({ causes, estimated, trends, onDrill }: FlowCausesProps) {
  const [sort, setSort] = useState<Sort>(DEFAULT_SORT);
  const prFixSeparable = !estimated.includes(PR_FIX_UNAVAILABLE);
  const label = (cause: string) => causeLabel(cause, prFixSeparable);
  const rows = sortRows(causes.rework, sort, label);
  const total = causes.rework.reduce((sum, row) => sum + (row.count?.value ?? 0), 0);

  const onSort = (key: SortKey) =>
    setSort((current) => (current.key === key ? { key, dir: current.dir === "ascending" ? "descending" : "ascending" } : { key, dir: FIRST_DIRECTION[key] }));

  if (total === 0) {
    return <p>No rework in this window ({plural(causes.mergedCount, "merged item")})</p>;
  }
  return (
    <div className={styles.scrollRegion} role="region" aria-label="Rework causes" tabIndex={0}>
      <table className={styles.table}>
        <caption className={styles.caption}>Rework causes, ranked by count</caption>
        <thead>
          <tr>
            <SortHeader sortKey="cause" sort={sort} onSort={onSort}>
              Cause
            </SortHeader>
            <SortHeader sortKey="count" sort={sort} onSort={onSort}>
              Count
            </SortHeader>
            <th scope="col" className={styles.cell}>
              Items affected
            </th>
            <th scope="col" className={styles.cell}>
              Trend (weekly)
            </th>
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row.cause}>
              <th scope="row" className={styles.cell}>
                {label(row.cause)}
              </th>
              <td className={styles.cell}>
                <DrillButton drillKey={row.count?.drillKey ?? ""} onDrill={onDrill} label={String(row.count?.value ?? 0)}>
                  {formatInt(row.count?.value ?? 0)}
                </DrillButton>
              </td>
              <td className={styles.cell}>
                <DrillButton drillKey={row.count?.drillKey ?? ""} onDrill={onDrill} label={`${row.itemsAffected} items affected`}>
                  {formatInt(row.itemsAffected)}
                </DrillButton>
              </td>
              <td className={styles.cell}>
                <CauseSparkline trends={trends} cause={row.cause} label={label(row.cause)} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
