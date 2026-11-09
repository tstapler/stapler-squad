import type { FlowWindow } from "@/lib/hooks/useBacklogFlow";

const DRILL_TITLES: Record<string, string> = {
  "rework.review_fail": "Review-FAIL rework",
  "rework.bounce": "Bounce rework",
  "rework.pr_fix": "PR-fix rework",
  "rework.other": "Other rework",
  "thrash.episode": "Thrashing items",
  "fpy.first_pass": "First-pass items",
  "fpy.thrash_affected": "Thrash-affected items",
  "tiles.lead_time": "Lead-time items",
  "tiles.merged": "Merged items",
  "pr.fix_dwell": "PR-fix dwell",
};

// Mirrors the dynamic key families in session/flow/drill.go.
const DYNAMIC_TITLES: readonly (readonly [prefix: string, title: (suffix: string) => string])[] = [
  ["funnel.stage.", (stage) => `Items that entered ${stage}`],
  ["review.class.", (cls) => `Review verdicts: ${cls}`],
  ["stuck.", (reason) => `Stuck: ${reason}`],
];

/** Human name for a drill key; an unregistered key falls back to the key itself. */
export function drillTitle(key: string): string {
  const fixed = DRILL_TITLES[key];
  if (fixed) return fixed;
  const match = DYNAMIC_TITLES.find(([prefix]) => key.startsWith(prefix));
  return match ? match[1](key.slice(match[0].length)) : key;
}

export const windowPhrase = (window: FlowWindow): string => (window === "all" ? "all time" : `last ${window}d`);
