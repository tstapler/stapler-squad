"use client";

import type { CompareFlowWindowsResponse, FlowSummary } from "@/gen/session/v1/backlog_flow_pb";
import { FlowFunnel } from "./FlowFunnel";
import { FlowTiles } from "./FlowTiles";

export type FlowSectionBodyId = "tiles" | "leaks" | "causes" | "review" | "pr" | "stuck" | "anomalies" | "notes";

export interface FlowSectionBodyProps {
  id: FlowSectionBodyId;
  summary: FlowSummary;
  comparison: CompareFlowWindowsResponse | null;
  /** Every number is a button that sets the `drill` URL parameter through this. */
  onDrill: (key: string) => void;
}

/** Maps a section id to its component; the page owns status, skeleton and error blocks. */
export function FlowSectionBody({ id, summary, comparison, onDrill }: FlowSectionBodyProps) {
  switch (id) {
    case "tiles":
      return <FlowTiles summary={summary} comparison={comparison} onDrill={onDrill} />;
    case "leaks":
      return summary.funnel ? (
        <FlowFunnel funnel={summary.funnel} collapsedEvents={summary.notes?.collapsedEvents ?? 0} onDrill={onDrill} />
      ) : null;
    default:
      return null;
  }
}
