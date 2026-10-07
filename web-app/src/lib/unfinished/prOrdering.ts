import type { UserPR } from "@/gen/session/v1/types_pb";
import { prAttention } from "./prAttention";

export type PRSortBy =
  | "attention-first"
  | "updated-desc"
  | "updated-asc"
  | "repo"
  | "ci-status";

export interface PRGroup {
  /** `host/owner/repo`, lowercased host. */
  key: string;
  prs: UserPR[];
}

export function prKey(pr: Pick<UserPR, "host" | "owner" | "repo" | "number">): string {
  return `${repoKey(pr)}#${pr.number}`;
}

export function repoKey(pr: Pick<UserPR, "host" | "owner" | "repo">): string {
  return `${(pr.host || "github.com").toLowerCase()}/${pr.owner}/${pr.repo}`;
}

/** 3 = failing checks or conflict, 2 = unresolved threads, 1 = changes requested only, 0 = none/draft. */
export function severityRank(pr: UserPR): 0 | 1 | 2 | 3 {
  if (pr.isDraft) return 0;
  const a = prAttention(pr);
  if (a.failingChecks > 0 || a.mergeConflict) return 3;
  if (a.unresolvedThreads > 0) return 2;
  if (a.changesRequested) return 1;
  return 0;
}

/** Lower = worse. */
function ciRank(pr: UserPR): number {
  if (pr.checkConclusion === "failure" || pr.checkConclusion === "error") return 0;
  if (pr.changesReqCount > 0) return 1;
  if (pr.checkConclusion === "success") return 3;
  return 2;
}

function updatedSeconds(pr: UserPR): number {
  return Number(pr.updatedAt?.seconds ?? 0n);
}

type Compare = (a: UserPR, b: UserPR) => number;

const byUpdatedDesc: Compare = (a, b) => updatedSeconds(b) - updatedSeconds(a);
const byUpdatedAsc: Compare = (a, b) => updatedSeconds(a) - updatedSeconds(b);
const then = (...cmps: Compare[]): Compare => (a, b) => {
  for (const c of cmps) {
    const r = c(a, b);
    if (r !== 0) return r;
  }
  return 0;
};

interface SortRule {
  card: Compare;
  /** Compares two groups given their already-sorted members. */
  group: (a: PRGroup, b: PRGroup) => number;
}

const maxOf = (g: PRGroup, f: (p: UserPR) => number) => Math.max(...g.prs.map(f));
const minOf = (g: PRGroup, f: (p: UserPR) => number) => Math.min(...g.prs.map(f));
const keyAsc = (a: PRGroup, b: PRGroup) => a.key.localeCompare(b.key);

const RULES: Record<PRSortBy, SortRule> = {
  "attention-first": {
    card: then((a, b) => severityRank(b) - severityRank(a), byUpdatedDesc),
    group: (a, b) =>
      maxOf(b, severityRank) - maxOf(a, severityRank) ||
      maxOf(b, updatedSeconds) - maxOf(a, updatedSeconds) ||
      keyAsc(a, b),
  },
  repo: {
    card: then((a, b) => severityRank(b) - severityRank(a), byUpdatedDesc),
    group: keyAsc,
  },
  "updated-desc": {
    card: byUpdatedDesc,
    group: (a, b) => maxOf(b, updatedSeconds) - maxOf(a, updatedSeconds) || keyAsc(a, b),
  },
  "updated-asc": {
    card: byUpdatedAsc,
    group: (a, b) => minOf(a, updatedSeconds) - minOf(b, updatedSeconds) || keyAsc(a, b),
  },
  "ci-status": {
    card: then((a, b) => ciRank(a) - ciRank(b), byUpdatedDesc),
    group: (a, b) => minOf(a, ciRank) - minOf(b, ciRank) || keyAsc(a, b),
  },
};

/** Groups PRs by repo and orders groups and cards per the sort's documented rule. Pure. */
export function orderPRs(prs: readonly UserPR[], sortBy: PRSortBy): PRGroup[] {
  const rule = RULES[sortBy];
  const map = new Map<string, UserPR[]>();
  for (const pr of prs) {
    const key = repoKey(pr);
    const list = map.get(key);
    if (list) list.push(pr);
    else map.set(key, [pr]);
  }
  const groups: PRGroup[] = [];
  for (const [key, members] of map) {
    groups.push({ key, prs: [...members].sort(rule.card) });
  }
  return groups.sort(rule.group);
}
