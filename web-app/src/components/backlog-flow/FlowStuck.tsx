"use client";

import { AppLink } from "@/components/ui/AppLink";
import { EstimatedValue } from "@/components/ui/EstimatedValue";
import { srOnly } from "@/components/ui/LiveRegion.css";
import type { FlowStuckReasonRow, FlowStuckSection } from "@/gen/session/v1/backlog_flow_pb";
import { DrillButton } from "./DrillButton";
import { durationText, formatInt, formatIsoDate, plural } from "./flowFormatters";
import * as styles from "./flow.css";

interface FlowStuckProps {
  stuck: FlowStuckSection;
  /** Estimated codes of the stuck section. */
  estimated: readonly string[];
  captureSince: Date | null;
  onDrill: (key: string) => void;
}

const LOWER_BOUND = "stuck_history:lower_bound";
const LOWER_BOUND_TITLE = "Stuck history before capture began is incomplete";

function DurationCell({ row, lowerBound }: { row: FlowStuckReasonRow; lowerBound: boolean }) {
  const text = durationText(row.duration);
  if (!text) return <span className={styles.muted}>n/a</span>;
  const value = `median ${text.median} / p90 ${text.p90}`;
  return lowerBound ? <EstimatedValue title={LOWER_BOUND_TITLE}>{value}</EstimatedValue> : <>{value}</>;
}

function Row({ row, lowerBound, onDrill }: { row: FlowStuckReasonRow; lowerBound: boolean; onDrill: (key: string) => void }) {
  return (
    <tr>
      <th scope="row" className={styles.cell}>
        {row.reason}
      </th>
      <td className={styles.cell}>
        <AppLink href="/unfinished" className={styles.link}>
          {formatInt(row.open)}
          <span className={srOnly}> open {row.reason} (opens Unfinished page)</span>
        </AppLink>
      </td>
      <td className={styles.cell}>
        <DrillButton drillKey={row.duration?.drillKey ?? ""} onDrill={onDrill}>
          {formatInt(row.duration?.n ?? 0)}
        </DrillButton>
      </td>
      <td className={styles.cell}>
        <DurationCell row={row} lowerBound={lowerBound || row.lowerBound} />
      </td>
    </tr>
  );
}

export function FlowStuck({ stuck, estimated, captureSince, onDrill }: FlowStuckProps) {
  const lowerBound = estimated.includes(LOWER_BOUND);
  const episodes = stuck.reasons.reduce((sum, row) => sum + (row.duration?.n ?? 0), 0);

  return (
    <div className={styles.block} data-testid="flow-stuck">
      {stuck.open === 0 && <p>Nothing stuck now; {plural(episodes, "episode")} historically</p>}
      {lowerBound && (
        <p className={styles.muted}>
          <span>history{captureSince ? ` before ${formatIsoDate(captureSince)}` : ""} is a lower bound</span>{" "}
          <EstimatedValue title={LOWER_BOUND_TITLE}>estimated</EstimatedValue>
        </p>
      )}
      {stuck.reasons.length > 0 && (
        <div className={styles.scrollRegion} role="region" aria-label="Stuck reasons" tabIndex={0}>
          <table className={styles.table}>
            <caption className={styles.caption}>Stuck reasons, current and historical</caption>
            <thead>
              <tr>
                {["Reason", "Open now", "Historical episodes", "Time in stuck"].map((heading) => (
                  <th key={heading} scope="col" className={styles.cell}>
                    {heading}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {stuck.reasons.map((row) => (
                <Row key={row.reason} row={row} lowerBound={lowerBound} onDrill={onDrill} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
