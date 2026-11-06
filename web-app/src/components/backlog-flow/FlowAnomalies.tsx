"use client";

import { timestampDate } from "@bufbuild/protobuf/wkt";
import { AppLink } from "@/components/ui/AppLink";
import { srOnly } from "@/components/ui/LiveRegion.css";
import type { FlowAnomaliesSection, FlowThrashItemRow } from "@/gen/session/v1/backlog_flow_pb";
import { formatAge } from "@/app/backlog/flow/flowFormat";
import { FLOW_POLL_INTERVAL_MS } from "@/lib/hooks/useBacklogFlow";
import { DrillButton } from "./DrillButton";
import { formatDurationMs, formatInt } from "./flowFormatters";
import * as styles from "./flow.css";

interface FlowAnomaliesProps {
  anomalies: FlowAnomaliesSection;
  archivedIds: ReadonlySet<string>;
  /** Client clock; a parameter so tests do not depend on the wall clock. */
  now?: number;
  onDrill: (key: string) => void;
  /** Opens the item's collapsed timeline; `invoker` is where focus returns when it closes. */
  onViewEvents?: (itemId: string, invoker: HTMLElement) => void;
}

const SHORT_ID_LENGTH = 8;
const COLUMNS = ["Item", "Flip pair", "Episodes", "Total events", "Duration", "Last seen", "Action"] as const;

interface RowProps {
  row: FlowThrashItemRow;
  archived: boolean;
  now: number;
  drillKey: string;
  onDrill: (key: string) => void;
  onViewEvents?: FlowAnomaliesProps["onViewEvents"];
}

function Row({ row, archived, now, drillKey, onDrill, onViewEvents }: RowProps) {
  const first = row.firstAt ? timestampDate(row.firstAt).getTime() : null;
  const last = row.lastAt ? timestampDate(row.lastAt).getTime() : null;
  const active = last !== null && now - last < FLOW_POLL_INTERVAL_MS;
  return (
    <tr>
      <th scope="row" className={styles.cell}>
        {row.title} <span className={styles.muted}>{row.itemId.slice(0, SHORT_ID_LENGTH)}</span>
        {archived && <span className={styles.chip}> archived</span>}
      </th>
      <td className={styles.cell}>{row.pair}</td>
      <td className={styles.cell}>
        <DrillButton drillKey={drillKey} onDrill={onDrill}>
          {formatInt(row.episodes)}
        </DrillButton>
      </td>
      <td className={styles.cell}>
        <DrillButton drillKey={drillKey} onDrill={onDrill}>
          {formatInt(row.events)}
        </DrillButton>
      </td>
      <td className={styles.cell}>{first !== null && last !== null ? formatDurationMs(last - first) : "n/a"}</td>
      <td className={styles.cell}>
        {last === null ? "n/a" : formatAge(now - last)}
        {active && <span className={styles.chip}> Still active</span>}
      </td>
      <td className={styles.cell}>
        <AppLink href={`?item=${row.itemId}`} className={styles.link}>
          Open item<span className={srOnly}> {row.title}</span>
        </AppLink>
        {onViewEvents && (
          <button type="button" className={styles.textButton} onClick={(event) => onViewEvents(row.itemId, event.currentTarget)}>
            View events<span className={srOnly}> {row.title}</span>
          </button>
        )}
      </td>
    </tr>
  );
}

/** One row per thrashing item (episodes merge across gaps, so an item still has several). */
export function FlowAnomalies({ anomalies, archivedIds, now = Date.now(), onDrill, onViewEvents }: FlowAnomaliesProps) {
  const items = anomalies.items?.items ?? [];
  if (items.length === 0) return <p>No thrash episodes in this window</p>;
  const total = Math.max(anomalies.items?.totalCount ?? 0, items.length);
  const drillKey = anomalies.thrashItems?.drillKey ?? "";
  return (
    <div className={styles.block}>
      <div className={styles.scrollRegion} role="region" aria-label="Anomalies and thrash" tabIndex={0}>
        <table className={styles.table}>
          <caption className={styles.caption}>Thrashing items, one row per item</caption>
          <thead>
            <tr>
              {COLUMNS.map((column) => (
                <th key={column} scope="col" className={styles.cell}>
                  {column}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {items.map((row) => (
              <Row key={row.itemId} row={row} archived={row.archived || archivedIds.has(row.itemId)} now={now} drillKey={drillKey} onDrill={onDrill} onViewEvents={onViewEvents} />
            ))}
          </tbody>
        </table>
      </div>
      {total > items.length && <span className={styles.muted}>Showing {items.length} of {total}</span>}
      <span className={styles.muted}>Excluded from aggregates: raw dwell and rework totals use collapsed episodes.</span>
    </div>
  );
}
