"use client";

import { timestampDate } from "@bufbuild/protobuf/wkt";
import type { CompareFlowWindowsResponse, FlowSummary } from "@/gen/session/v1/backlog_flow_pb";
import { FlowAnomalies } from "./FlowAnomalies";
import { FlowCauses } from "./FlowCauses";
import { FlowDataNotes } from "./FlowDataNotes";
import { FlowFunnel } from "./FlowFunnel";
import { FlowPR } from "./FlowPR";
import { FlowReview } from "./FlowReview";
import { FlowStuck } from "./FlowStuck";
import { FlowTiles } from "./FlowTiles";

export type FlowSectionBodyId = "tiles" | "leaks" | "causes" | "review" | "pr" | "stuck" | "anomalies" | "notes";

export interface FlowSectionBodyProps {
  id: FlowSectionBodyId;
  summary: FlowSummary;
  comparison: CompareFlowWindowsResponse | null;
  /** Every number is a button that sets the `drill` URL parameter through this. */
  onDrill: (key: string) => void;
}

/** The response does not flag archived thrashers yet; callers with that knowledge pass their own ids. */
export const NO_ARCHIVED_IDS: ReadonlySet<string> = new Set();

const captureSince = (summary: FlowSummary): Date | null => (summary.meta?.captureSince ? timestampDate(summary.meta.captureSince) : null);

/** Maps a section id to its component; the page owns status, skeleton and error blocks. */
export function FlowSectionBody({ id, summary, comparison, onDrill }: FlowSectionBodyProps) {
  switch (id) {
    case "tiles":
      return <FlowTiles summary={summary} comparison={comparison} onDrill={onDrill} />;
    case "leaks":
      return summary.funnel ? (
        <FlowFunnel funnel={summary.funnel} collapsedEvents={summary.notes?.collapsedEvents ?? 0} onDrill={onDrill} />
      ) : null;
    case "causes":
      return summary.causes ? (
        <FlowCauses causes={summary.causes} estimated={summary.causes.meta?.estimated ?? []} trends={summary.trends} onDrill={onDrill} />
      ) : null;
    case "review":
      return summary.review ? (
        <FlowReview review={summary.review} estimated={summary.review.meta?.estimated ?? []} trends={summary.trends} onDrill={onDrill} />
      ) : null;
    case "pr":
      return summary.pr ? (
        <FlowPR pr={summary.pr} causes={summary.causes} estimated={summary.pr.meta?.estimated ?? []} captureSince={captureSince(summary)} onDrill={onDrill} />
      ) : null;
    case "stuck":
      return summary.stuck ? (
        <FlowStuck stuck={summary.stuck} estimated={summary.stuck.meta?.estimated ?? []} captureSince={captureSince(summary)} onDrill={onDrill} />
      ) : null;
    case "anomalies":
      return summary.anomalies ? <FlowAnomalies anomalies={summary.anomalies} archivedIds={NO_ARCHIVED_IDS} onDrill={onDrill} /> : null;
    case "notes":
      return <FlowDataNotes notes={summary.notes} meta={summary.meta} />;
    default:
      return null;
  }
}
