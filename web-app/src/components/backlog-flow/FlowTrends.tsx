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
}

/** A weekly line chart with its first and last values printed, so the trend never depends on the drawing. */
function FlowSparkline({ testId, caption, weeks, values }: SparklineProps) {
  const first = values[0];
  const last = values[values.length - 1];
  const takeaway = `${caption}: ${formatInt(first)} in the week of ${weeks[0]}, ${formatInt(last)} in the week of ${weeks[weeks.length - 1]}`;
  return (
    <div data-testid={testId}>
      <FlowChart caption={caption} takeaway={takeaway} categories={weeks} series={[{ name: caption, values }]} kind="line" width={SPARK_WIDTH} height={SPARK_HEIGHT} />
      <span className={styles.muted}>
        first week {formatInt(first)}, last week {formatInt(last)}
      </span>
    </div>
  );
}

export interface TrendsProps {
  trends: FlowTrendsSection | undefined;
}

export function ThroughputSparkline({ trends }: TrendsProps) {
  const buckets = trends?.throughput ?? [];
  if (buckets.length === 0) return null;
  return (
    <FlowSparkline testId="flow-sparkline-throughput" caption="Merged items per week" weeks={buckets.map((b) => weekLabel(b.weekStart))} values={buckets.map((b) => b.n)} />
  );
}

export function CauseSparkline({ trends, cause, label }: TrendsProps & { cause: string; label: string }) {
  const buckets = trends?.rework ?? [];
  if (buckets.length === 0) return null;
  const values = buckets.map((b) => b.byCause.find((cell) => cell.name === cause)?.n ?? 0);
  return <FlowSparkline testId={`flow-sparkline-${cause}`} caption={`${label} per week`} weeks={buckets.map((b) => weekLabel(b.weekStart))} values={values} />;
}
