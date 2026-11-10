import type { FlowSectionStatus, FlowSummary } from "@/gen/session/v1/backlog_flow_pb";

export type FlowSectionId = "tiles" | "leaks" | "causes" | "review" | "pr" | "stuck" | "anomalies" | "notes";
type FlowSectionKey = "tiles" | "funnel" | "causes" | "review" | "pr" | "stuck" | "anomalies" | "notes";

export interface FlowSectionDef {
  /** Also the `id` of the section's `h2`, which the jump list scrolls to and focuses. */
  id: FlowSectionId;
  /** The response field that carries the section's status. */
  key: FlowSectionKey;
  title: string;
  /** Name used in "<name> section failed to load". */
  shortName: string;
  /** Lower sections (UX criterion 43) start closed and show a one-line summary. */
  collapsible: boolean;
  /** Heading is for assistive tech only; the content is self-explanatory (the tile row). */
  headingHidden?: boolean;
}

/** Page order; sections A to E are expanded, F to H collapsed. */
export const FLOW_SECTIONS: readonly FlowSectionDef[] = [
  { id: "tiles", key: "tiles", title: "Health summary", shortName: "Health summary", collapsible: false, headingHidden: true },
  { id: "leaks", key: "funnel", title: "Where does it leak?", shortName: "Funnel", collapsible: false },
  { id: "causes", key: "causes", title: "Why: rework causes", shortName: "Causes", collapsible: false },
  { id: "review", key: "review", title: "Review gate", shortName: "Review", collapsible: false },
  { id: "pr", key: "pr", title: "PR health", shortName: "PR health", collapsible: false },
  { id: "stuck", key: "stuck", title: "Stuck reasons", shortName: "Stuck reasons", collapsible: true },
  { id: "anomalies", key: "anomalies", title: "Anomalies & thrash", shortName: "Anomalies", collapsible: true },
  { id: "notes", key: "notes", title: "Data notes", shortName: "Data notes", collapsible: true },
];

export const FLOW_JUMP_LINKS: readonly { label: string; id: FlowSectionId }[] = [
  { label: "Leaks", id: "leaks" },
  { label: "Causes", id: "causes" },
  { label: "Review", id: "review" },
  { label: "PR", id: "pr" },
  { label: "Stuck", id: "stuck" },
  { label: "Anomalies", id: "anomalies" },
];

export function sectionStatus(summary: FlowSummary, def: FlowSectionDef): FlowSectionStatus | undefined {
  return summary[def.key]?.meta;
}

/** Failed and partial sections render expanded so a problem is never hidden behind a closed toggle. */
export function needsAttention(status: FlowSectionStatus | undefined): boolean {
  return status?.status === "failed" || status?.status === "partial";
}

const plural = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`;

/** The always-visible line of a collapsed section, or null when the section has nothing to summarise. */
export function sectionSummaryLine(id: FlowSectionId, summary: FlowSummary): string | null {
  if (id === "stuck" && summary.stuck) {
    const episodes = summary.stuck.reasons.reduce((total, reason) => total + (reason.duration?.n ?? 0), 0);
    return `${summary.stuck.open} open, ${plural(episodes, "historical episode")}`;
  }
  if (id === "anomalies" && summary.anomalies) {
    return `${plural(summary.anomalies.thrashItems?.value ?? 0, "item")} thrashing`;
  }
  return null;
}
