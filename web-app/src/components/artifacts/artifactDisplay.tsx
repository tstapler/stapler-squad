import type { ReactNode } from "react";
import { timestampMs } from "@bufbuild/protobuf/wkt";
import type { SessionArtifactHit, SessionArtifactRow } from "@/gen/session/v1/session_pb";
import { PullRequestChip } from "@/components/shared/PullRequestChip";
import { toItem } from "@/lib/artifacts/groupArtifacts";
import { routes } from "@/lib/routes";
import * as s from "./ArtifactsBrowser.css";

/** One list entry, whatever RPC produced it: a record (browse) or a session hit (search, PR lookup). */
export interface DisplayItem {
  key: string;
  kind: string;
  artifact: ReactNode;
  sessionId: string;
  sessionTitle: string;
  archived: boolean;
  whenMs: number | undefined;
  /** "Tracked" or "Seen in transcript"; only PR lookups and PR rows carry one. */
  sourceLabel?: string;
}

export const sourceLabelOf = (source: string): string => (source === "session_pr" ? "Tracked" : "Seen in transcript");

/** The session detail opened on its Artifacts tab (the row link target, UX-S8 step 4). */
export const sessionArtifactsHref = (sessionId: string): string => `${routes.sessionDetail(sessionId)}&tab=artifacts`;

const KIND_LABELS: Record<string, string> = { pr: "PR", commit: "commit", branch: "branch", issue: "issue", plan_doc: "plan", ci_link: "CI", command: "command" };
export const kindLabelOf = (kind: string): string => KIND_LABELS[kind] ?? kind;

function recordArtifact(row: SessionArtifactRow): ReactNode {
  const rec = row.record;
  if (!rec) return null;
  if (rec.record.case === "pr" && row.pullRequest) {
    const pr = row.pullRequest;
    return (
      <span>
        <PullRequestChip pullRequests={[pr]} onOpenList={() => {}} />{" "}
        <span className={s.mono}>{pr.owner}/{pr.repo}#{pr.number}</span>
      </span>
    );
  }
  if (rec.record.case === "command") return <span className={s.mono}>{rec.record.value.command}</span>;
  const item = toItem(rec, 0);
  if (!item) return <span className={s.muted}>Unsupported record</span>;
  if (item.redacted) return <span className={s.mono}>[redacted]</span>;
  return (
    <span>
      <span className={s.mono}>{item.title}</span>
      {item.meta && <span className={s.muted}> {item.meta}</span>}
    </span>
  );
}

export function itemsFromRows(rows: readonly SessionArtifactRow[]): DisplayItem[] {
  return rows.map((row, i) => ({
    key: `${row.sessionId}-${i}`,
    kind: row.record?.kind ?? "",
    artifact: recordArtifact(row),
    sessionId: row.sessionId,
    sessionTitle: row.sessionTitle,
    archived: row.sessionArchived,
    whenMs: row.record?.firstSeenAt ? timestampMs(row.record.firstSeenAt) : row.sessionLastActivity ? timestampMs(row.sessionLastActivity) : undefined,
    sourceLabel: row.record?.record.case === "pr" ? sourceLabelOf(row.source) : undefined,
  }));
}

export function itemsFromHits(hits: readonly SessionArtifactHit[]): DisplayItem[] {
  return hits.map((hit) => ({
    key: hit.sessionId,
    kind: hit.matches[0]?.type ?? "",
    artifact: hit.matches.length === 0 ? <span className={s.muted}>No matched artifact</span> : (
      <ul className={s.cards}>
        {hit.matches.slice(0, 3).map((m) => (
          <li key={`${m.type}-${m.text}`}><span className={s.kind}>{kindLabelOf(m.type)}</span> <span className={s.mono}>{m.text}</span></li>
        ))}
      </ul>
    ),
    sessionId: hit.sessionId,
    sessionTitle: hit.title,
    archived: false,
    whenMs: hit.lastActivity ? timestampMs(hit.lastActivity) : undefined,
    sourceLabel: sourceLabelOf(hit.source),
  }));
}
