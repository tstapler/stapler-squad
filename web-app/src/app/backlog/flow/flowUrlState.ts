import type { FlowWindow } from "@/lib/hooks/useBacklogFlow";
import type { NavSearchParams } from "@/lib/nav/impl.types";

export const DEFAULT_FLOW_WINDOW: FlowWindow = 30;
export const BOUNDED_WINDOWS = [7, 30, 90] as const;

export type FlowCompare = "none" | "prev";
export type FlowUrlNotice = "unsupported-window" | "unknown-drill";

export interface FlowUrlState {
  window: FlowWindow;
  compare: FlowCompare;
  /** A drill key the server knows; unknown keys are dropped with a notice. */
  drill: string | null;
  notices: FlowUrlNotice[];
  /** The query string with every invalid or impossible parameter removed. */
  cleanSearch: string;
  /** True when the URL carried something `cleanSearch` removes, so the page should replace it. */
  needsRewrite: boolean;
}

// Mirrors session/flow/drill.go: the static keys plus the dynamic families that take a suffix.
const STATIC_DRILL_KEYS = new Set([
  "rework.review_fail",
  "rework.bounce",
  "rework.pr_fix",
  "rework.other",
  "thrash.episode",
  "fpy.first_pass",
  "fpy.thrash_affected",
  "tiles.lead_time",
  "tiles.merged",
  "pr.fix_dwell",
]);
const DYNAMIC_DRILL_PREFIXES = ["funnel.stage.", "review.class.", "stuck."];

export function isKnownDrillKey(key: string): boolean {
  return (
    STATIC_DRILL_KEYS.has(key) ||
    DYNAMIC_DRILL_PREFIXES.some((prefix) => key.startsWith(prefix) && key.length > prefix.length)
  );
}

function parseWindow(raw: string | null): { window: FlowWindow; unsupported: boolean } {
  if (raw === null) return { window: DEFAULT_FLOW_WINDOW, unsupported: false };
  if (raw === "all") return { window: "all", unsupported: false };
  const days = BOUNDED_WINDOWS.find((candidate) => String(candidate) === raw);
  return days ? { window: days, unsupported: false } : { window: DEFAULT_FLOW_WINDOW, unsupported: true };
}

export function parseFlowUrlState(params: NavSearchParams): FlowUrlState {
  const clean = new URLSearchParams(params.toString());
  const notices: FlowUrlNotice[] = [];

  const { window, unsupported } = parseWindow(params.get("window"));
  if (unsupported) {
    notices.push("unsupported-window");
    clean.delete("window");
  }

  // `previous period` is undefined for all time, so the parameter cannot stay.
  const compare: FlowCompare = params.get("compare") === "prev" && window !== "all" ? "prev" : "none";
  if (params.has("compare") && compare === "none") clean.delete("compare");

  let drill: string | null = null;
  const rawDrill = params.get("drill");
  if (rawDrill !== null) {
    if (isKnownDrillKey(rawDrill)) drill = rawDrill;
    else {
      notices.push("unknown-drill");
      clean.delete("drill");
    }
  }

  const cleanSearch = clean.toString();
  return { window, compare, drill, notices, cleanSearch, needsRewrite: cleanSearch !== params.toString() };
}

export interface FlowSearchPatch {
  window?: FlowWindow;
  compare?: FlowCompare;
  drill?: string | null;
}

/** The next query string for a user change; other parameters (such as `item`) are kept. */
export function buildFlowSearch(current: NavSearchParams, patch: FlowSearchPatch): string {
  const next = new URLSearchParams(current.toString());
  if (patch.window !== undefined) {
    next.set("window", String(patch.window));
    // All time has no previous period, so it cannot keep a comparison.
    if (patch.window === "all") next.delete("compare");
  }
  if (patch.compare !== undefined) {
    if (patch.compare === "prev") next.set("compare", "prev");
    else next.delete("compare");
  }
  if (patch.drill !== undefined) {
    if (patch.drill === null) next.delete("drill");
    else next.set("drill", patch.drill);
  }
  const query = next.toString();
  return query === "" ? "" : `?${query}`;
}
