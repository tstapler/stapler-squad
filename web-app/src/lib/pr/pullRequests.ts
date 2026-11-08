import { timestampMs } from "@bufbuild/protobuf/wkt";
import type { PrHold, PullRequestRef, SessionPullRequest } from "@/gen/session/v1/types_pb";

/** Visible state of one PR row. `unverified` is a backfilled terminal row no poll has confirmed yet. */
export type PrRowState = "open" | "draft" | "merged" | "closed" | "unknown" | "unverified";

/** Terminal rows beyond this many collapse behind "N earlier PRs" on desktop (ux.md S2). */
export const TERMINAL_ROWS_BEFORE_COLLAPSE = 3;
/** Terminal rows still shown while the rest are collapsed. */
export const TERMINAL_ROWS_WHEN_COLLAPSED = 2;
/** Row cap on a phone before "Show all (N)". */
export const COMPACT_ROW_CAP = 5;

const DEFAULT_HOST = "github.com";

export function prHost(pr: Pick<PullRequestRef, "host">): string {
  return pr.host || DEFAULT_HOST;
}

export function prKey(pr: Pick<PullRequestRef, "host" | "owner" | "repo" | "number">): string {
  return `${prHost(pr)}/${pr.owner}/${pr.repo}#${pr.number}`;
}

/**
 * The PR's GitHub URL. The list-response form of SessionPullRequest omits `url` (K11), so derive it
 * from host/owner/repo/number rather than render an empty href that opens about:blank.
 */
export function prUrl(pr: Pick<PullRequestRef, "host" | "owner" | "repo" | "number"> & { url?: string }): string {
  return pr.url || `https://${prHost(pr)}/${pr.owner}/${pr.repo}/pull/${pr.number}`;
}

/** "#12 acme/app": how every action names its PR (ux.md target phrase). */
export function prTarget(pr: Pick<PullRequestRef, "owner" | "repo" | "number">): string {
  return `#${pr.number} ${pr.owner}/${pr.repo}`;
}

export function toRef(pr: SessionPullRequest): Pick<PullRequestRef, "host" | "owner" | "repo" | "number"> {
  return { host: pr.host, owner: pr.owner, repo: pr.repo, number: pr.number };
}

export function prRowState(pr: SessionPullRequest): PrRowState {
  if (pr.outcomeUnverified) return "unverified";
  if (pr.state === "merged" || pr.state === "closed") return pr.state;
  if (pr.state === "open") return pr.isDraft ? "draft" : "open";
  return "unknown";
}

export function isTerminal(pr: SessionPullRequest): boolean {
  const s = prRowState(pr);
  return s === "merged" || s === "closed" || s === "unverified";
}

/** The server's own `is_active` rule; never recomputed here. */
export function activePullRequest(prs: readonly SessionPullRequest[]): SessionPullRequest | undefined {
  return prs.find((p) => p.isActive);
}

export function openPullRequests(prs: readonly SessionPullRequest[]): SessionPullRequest[] {
  return prs.filter((p) => !isTerminal(p) && prRowState(p) !== "unknown");
}

function recency(pr: SessionPullRequest): number {
  const ts = pr.mergedAt ?? pr.closedAt ?? pr.createdAt;
  return ts ? timestampMs(ts) : 0;
}

/** Active first, then non-terminal by recency, then terminal by recency. */
export function sortPullRequests(prs: readonly SessionPullRequest[]): SessionPullRequest[] {
  const rank = (p: SessionPullRequest) => (p.isActive ? 0 : isTerminal(p) ? 2 : 1);
  return [...prs].sort((a, b) => rank(a) - rank(b) || recency(b) - recency(a) || b.number - a.number);
}

export interface RowPlan {
  /** Rows to render, in order. */
  visible: SessionPullRequest[];
  /** Rows folded away; 0 when the list shows everything. */
  hidden: number;
  /** Which control folds them: "N earlier PRs" (desktop) or "Show all (N)" (phone). */
  disclosure: "earlier" | "showAll" | null;
}

/** Decides which rows show. `expanded` reveals them all; `compact` is the phone row cap. */
export function planRows(prs: readonly SessionPullRequest[], opts: { expanded: boolean; compact: boolean }): RowPlan {
  const sorted = sortPullRequests(prs);
  if (opts.compact) {
    const hidden = Math.max(0, sorted.length - COMPACT_ROW_CAP);
    if (hidden === 0) return { visible: sorted, hidden: 0, disclosure: null };
    return { visible: opts.expanded ? sorted : sorted.slice(0, COMPACT_ROW_CAP), hidden, disclosure: "showAll" };
  }
  const terminal = sorted.filter((p) => isTerminal(p) && !p.isActive);
  if (terminal.length <= TERMINAL_ROWS_BEFORE_COLLAPSE) return { visible: sorted, hidden: 0, disclosure: null };
  const hiddenRows = new Set(terminal.slice(TERMINAL_ROWS_WHEN_COLLAPSED));
  return {
    visible: opts.expanded ? sorted : sorted.filter((p) => !hiddenRows.has(p)),
    hidden: hiddenRows.size,
    disclosure: "earlier",
  };
}

/** How many rows "N earlier PRs" folds on desktop. */
export function earlierCount(prs: readonly SessionPullRequest[]): number {
  return planRows(prs, { expanded: false, compact: false }).hidden;
}

/** The "+N" count beside a chip: every PR except the one the chip shows. */
export function moreCount(prs: readonly SessionPullRequest[]): number {
  return Math.max(0, prs.length - 1);
}

export function holdsFor(holds: readonly PrHold[], pr: SessionPullRequest): PrHold[] {
  const key = prKey(pr);
  return holds.filter((h) => h.pr && prKey(h.pr) === key);
}
