import { timestampMs } from "@bufbuild/protobuf/wkt";
import type { ArtifactPRRecord, ArtifactRecordProto, SessionArtifacts } from "@/gen/session/v1/types_pb";

export type ArtifactItemKind = "commit" | "branch" | "issue" | "plan_doc" | "ci_link";

export interface ArtifactItem {
  key: string;
  kind: ArtifactItemKind;
  /** Human kind word, also the start of the copy button's name. */
  kindLabel: string;
  title: string;
  meta: string;
  href?: string;
  /** What the copy button copies; absent for links and for redacted values. */
  copyValue?: string;
  copyName?: string;
  redacted: boolean;
  atMs?: number;
}

export interface PrGroup {
  key: string;
  host: string;
  owner: string;
  repo: string;
  number: number;
  url: string;
  createdAtMs?: number;
  items: ArtifactItem[];
}

export interface MentionedPr {
  key: string;
  number: number;
  owner: string;
  repo: string;
  url: string;
}

export interface ArtifactGrouping {
  prGroups: PrGroup[];
  unassigned: ArtifactItem[];
  mentioned: MentionedPr[];
  /** Records this version cannot render (empty oneof, missing key fields). */
  skipped: number;
}

export const REDACTED = "[redacted]";
export const isRedacted = (value: string): boolean => value.includes(REDACTED);

const prUrl = (pr: Pick<ArtifactPRRecord, "host" | "owner" | "repo" | "number">) =>
  `https://${pr.host || "github.com"}/${pr.owner}/${pr.repo}/pull/${pr.number}`;
const prKeyOf = (pr: Pick<ArtifactPRRecord, "host" | "owner" | "repo" | "number">) => `${pr.host || "github.com"}/${pr.owner}/${pr.repo}#${pr.number}`;
const shortSha = (sha: string) => sha.slice(0, 7);

function item(base: Omit<ArtifactItem, "redacted" | "copyValue" | "copyName"> & { copyValue?: string; copyName?: string }): ArtifactItem {
  const redacted = isRedacted(base.title) || isRedacted(base.href ?? "") || isRedacted(base.copyValue ?? "");
  return { ...base, redacted, copyValue: redacted ? undefined : base.copyValue, copyName: redacted ? undefined : base.copyName };
}

/** A non-PR record as a displayable item (null for PR and unknown records). */
export function toItem(rec: ArtifactRecordProto, index: number): ArtifactItem | null {
  const atMs = rec.firstSeenAt ? timestampMs(rec.firstSeenAt) : undefined;
  const key = `${rec.kind}-${index}`;
  const r = rec.record;
  switch (r.case) {
    case "commit":
      if (!r.value.sha) return null;
      return item({ key, kind: "commit", kindLabel: "commit", title: shortSha(r.value.sha), meta: [r.value.subject, r.value.branch].filter(Boolean).join("  "), copyValue: r.value.sha, copyName: `Copy commit SHA ${shortSha(r.value.sha)}`, atMs });
    case "branch":
      if (!r.value.name) return null;
      return item({ key, kind: "branch", kindLabel: "branch", title: r.value.name, meta: r.value.action, copyValue: r.value.name, copyName: `Copy branch name ${r.value.name}`, atMs });
    case "planDoc":
      if (!r.value.path) return null;
      return item({ key, kind: "plan_doc", kindLabel: "plan", title: r.value.path, meta: "", copyValue: r.value.path, copyName: `Copy plan path ${r.value.path}`, atMs });
    case "issue": {
      const i = r.value;
      if (!i.owner || !i.number) return null;
      return item({ key, kind: "issue", kindLabel: "issue", title: `#${i.number} ${i.owner}/${i.repo}`, meta: i.relation, href: `https://${i.host || "github.com"}/${i.owner}/${i.repo}/issues/${i.number}`, atMs });
    }
    case "ciLink":
      if (!r.value.url) return null;
      return item({ key, kind: "ci_link", kindLabel: "CI", title: r.value.url, meta: "", href: r.value.url, atMs });
    default:
      return null;
  }
}

/**
 * Groups v2 records by the PR the session created (ux.md S7). An item belongs to the latest created PR whose
 * create time is at or before the item's first-seen time; earlier or undated items are Unassigned. This is a
 * guess from timing alone, which the tab says once in its help line.
 */
export function groupArtifacts(artifacts: SessionArtifacts): ArtifactGrouping {
  const groups = new Map<string, PrGroup>();
  const mentioned = new Map<string, MentionedPr>();
  const items: ArtifactItem[] = [];
  let skipped = 0;

  artifacts.records.forEach((rec, index) => {
    if (rec.record.case === "pr") {
      const pr = rec.record.value;
      if (!pr.owner || !pr.repo || !pr.number) { skipped += 1; return; }
      const key = prKeyOf(pr);
      const base = { key, number: pr.number, owner: pr.owner, repo: pr.repo, url: prUrl(pr) };
      if (pr.source === "created") {
        if (!groups.has(key)) groups.set(key, { ...base, host: pr.host, createdAtMs: rec.firstSeenAt ? timestampMs(rec.firstSeenAt) : undefined, items: [] });
      } else if (!mentioned.has(key)) {
        mentioned.set(key, base);
      }
      return;
    }
    const built = toItem(rec, index);
    if (built) items.push(built); else skipped += 1;
  });
  for (const key of groups.keys()) mentioned.delete(key);

  const prGroups = [...groups.values()].sort((a, b) => (a.createdAtMs ?? Infinity) - (b.createdAtMs ?? Infinity));
  const dated = prGroups.filter((g) => g.createdAtMs !== undefined);
  const unassigned: ArtifactItem[] = [];
  for (const it of [...items].sort((a, b) => (a.atMs ?? Infinity) - (b.atMs ?? Infinity))) {
    const owner = it.atMs === undefined ? undefined : [...dated].reverse().find((g) => (g.createdAtMs as number) <= (it.atMs as number));
    (owner ? owner.items : unassigned).push(it);
  }
  return { prGroups, unassigned, mentioned: [...mentioned.values()], skipped };
}
