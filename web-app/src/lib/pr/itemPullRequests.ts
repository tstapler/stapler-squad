import type { SessionPullRequest } from "@/gen/session/v1/types_pb";
import { prRowState, type PrRowState } from "./pullRequests";

export interface ItemPrSummary {
  merged: number;
  closed: number;
  total: number;
  /** "2 of 3 merged": the primary signal; the bar only decorates it. */
  text: string;
  valueText: string;
  showBar: boolean;
  /** The done rule plus why the item is or is not done. */
  ruleLine: string;
}

const RULE = "Done when at least one PR is merged and none are open.";
const BLOCKING_WORD: Partial<Record<PrRowState, string>> = { open: "open", draft: "draft", unknown: "unknown", unverified: "unverified" };

/** "k of N merged" for a backlog item (ux.md S5). The server decides when the item is done; this only words it. */
export function itemPrSummary(prs: readonly SessionPullRequest[]): ItemPrSummary {
  const states = prs.map(prRowState);
  const total = prs.length;
  const merged = states.filter((s) => s === "merged").length;
  const closed = states.filter((s) => s === "closed").length;
  const blocking = prs.filter((_, i) => BLOCKING_WORD[states[i]]);

  let text = `${merged} of ${total} merged`;
  if (blocking.length === 0 && total > 0) {
    if (merged === 0) text = `0 merged, ${closed} closed. No merged PR, so the item can't complete.`;
    else if (closed > 0) text = `${merged} merged, ${closed} closed`;
  }
  const waiting = blocking.map((pr) => `#${pr.number} (${BLOCKING_WORD[prRowState(pr)]})`);
  const why = waiting.length > 0 ? ` Waiting on ${waiting.join(", ")}.` : merged > 0 ? " Every PR is resolved." : "";
  return { merged, closed, total, text, valueText: `${merged} of ${total} pull requests merged`, showBar: total > 1, ruleLine: `${RULE}${why}` };
}

/** Rows that can still hold a cleanup (open, draft, unknown, unverified) offer the item-level exit. */
export function canStopTracking(pr: SessionPullRequest): boolean {
  return BLOCKING_WORD[prRowState(pr)] !== undefined;
}
