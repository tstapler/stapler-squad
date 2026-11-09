"use client";

import { timestampDate } from "@bufbuild/protobuf/wkt";
import type { Timestamp } from "@bufbuild/protobuf/wkt";
import type { FlowTrendsSection } from "@/gen/session/v1/backlog_flow_pb";
import { FlowChart } from "./FlowChart";
import { formatInt, formatIsoDate } from "./flowFormatters";
import * as styles from "./flow.css";

const SPARK_WIDTH = 120;
const SPARK_HEIGHT = 32;

/** ISO week start (Monday, UTC) as `2026-10-05`. */
export const weekLabel = (start: Timestamp | undefined): string => (start ? formatIsoDate(timestampDate(start)) : "unknown week");

interface SparklineProps {
  testId: string;
  /** What the series counts, e.g. "Merged items per week". */
  caption: string;
  weeks: readonly string[];
  values: readonly number[];
  drillKeys?: readonly string[];
  onDrill?: (key: string) => void;
}

/** A weekly line chart with its first and last values printed, so the trend never depends on the drawing. */
function FlowSparkline({ testId, caption, weeks, values, drillKeys, onDrill }: SparklineProps) {
  const first = values[0];
  const last = values[values.length - 1];
  const takeaway = `${caption}: ${formatInt(first)} in the week of ${weeks[0]}, ${formatInt(last)} in the week of ${weeks[weeks.length - 1]}`;
  return (
    <div data-testid={testId}>
      <FlowChart caption={caption} takeaway={takeaway} categories={weeks} series={[{ name: caption, values }]} kind="line" width={SPARK_WIDTH} height={SPARK_HEIGHT} drillKeys={drillKeys} onDrill={onDrill} />
      <span className={styles.muted}>
        first week {formatInt(first)}, last week {formatInt(last)}
      </span>
    </div>
  );
}

export interface TrendsProps {
  trends: FlowTrendsSection | undefined;
  /** Opens the items behind a week; omitted means the sparkline is read-only. */
  onDrill?: (key: string) => void;
}

export function ThroughputSparkline({ trends, onDrill }: TrendsProps) {
  const buckets = trends?.throughput ?? [];
  if (buckets.length === 0) return null;
  return (
    <FlowSparkline testId="flow-sparkline-throughput" caption="Merged items per week" weeks={buckets.map((b) => weekLabel(b.weekStart))} values={buckets.map((b) => b.n)} drillKeys={buckets.map((b) => b.drillKey)} onDrill={onDrill} />
  );
}

export function CauseSparkline({ trends, cause, label, onDrill }: TrendsProps & { cause: string; label: string }) {
  const buckets = trends?.rework ?? [];
  if (buckets.length === 0) return null;
  const cells = buckets.map((b) => b.byCause.find((cell) => cell.name === cause));
  return (
    <FlowSparkline
      testId={`flow-sparkline-${cause}`}
      caption={`${label} per week`}
      weeks={buckets.map((b) => weekLabel(b.weekStart))}
      values={cells.map((cell) => cell?.n ?? 0)}
      drillKeys={cells.map((cell) => cell?.drillKey ?? "")}
      onDrill={onDrill}
    />
  );
}
