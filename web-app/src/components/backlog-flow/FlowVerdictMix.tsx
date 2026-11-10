"use client";

import type { FlowTrendsSection, FlowVerdictMixBucket } from "@/gen/session/v1/backlog_flow_pb";
import { FlowChart } from "./FlowChart";
import { weekLabel } from "./FlowTrends";

const OUTCOMES = ["PASS", "FAIL", "PARTIAL", "UNVERIFIABLE"] as const;
const CAPTION = "Review verdicts per week, all verdict classes";

const outcomeCount = (bucket: FlowVerdictMixBucket, outcome: string): number =>
  bucket.cells.filter((cell) => cell.outcome === outcome).reduce((sum, cell) => sum + cell.n, 0);

const failShare = (bucket: FlowVerdictMixBucket): number => Math.round((outcomeCount(bucket, "FAIL") / bucket.n) * 100);

/** Compares the first and last weeks that had verdicts; an empty week has no share to compare. */
function takeawayFor(buckets: readonly FlowVerdictMixBucket[]): string {
  const withVerdicts = buckets.filter((bucket) => bucket.n > 0);
  if (withVerdicts.length === 0) return `No review verdicts in ${buckets.length} weeks`;
  const first = failShare(withVerdicts[0]);
  const last = failShare(withVerdicts[withVerdicts.length - 1]);
  const span = `over ${buckets.length} weeks`;
  if (first === last) return `FAIL share held at ${first}% ${span}`;
  return `FAIL share ${last < first ? "fell" : "rose"} from ${first}% to ${last}% ${span}`;
}

/** Stacked weekly verdict outcomes; a week with no verdicts is a labelled zero, not a gap. */
export function FlowVerdictMix({ trends }: { trends: FlowTrendsSection | undefined }) {
  const buckets = trends?.verdictMix ?? [];
  if (buckets.length === 0) return null;
  return (
    <div data-testid="flow-verdict-mix">
      <FlowChart
        caption={CAPTION}
        takeaway={takeawayFor(buckets)}
        categories={buckets.map((bucket) => weekLabel(bucket.weekStart))}
        series={OUTCOMES.map((outcome) => ({ name: outcome, values: buckets.map((bucket) => outcomeCount(bucket, outcome)) }))}
        kind="stacked-bar"
        width={320}
        height={120}
      />
    </div>
  );
}
