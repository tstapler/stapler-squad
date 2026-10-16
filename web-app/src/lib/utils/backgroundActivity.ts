import { replyAvailability } from "@/lib/reply/replyQuestion";
import type { NotificationData, NotificationHistoryItem } from "@/lib/types/notification";

/** A hidden session as the Background section needs it. `id` is the title today (types.proto:11). */
export interface HiddenSessionRef {
  id: string;
  title: string;
}

export interface HiddenSessionSummary extends HiddenSessionRef {
  state: "running" | "stopped" | "other";
  updatedAtMs: number;
}

export type BackgroundRowKind = "failure" | "needs_human";
export type BackgroundStatusLabel = "FAILED" | "NEEDS INPUT" | "NEEDS APPROVAL";

export interface BackgroundRow {
  /** Stable React key: one row per hidden session. */
  key: string;
  sessionId: string;
  sessionTitle: string;
  kind: BackgroundRowKind;
  statusLabel: BackgroundStatusLabel;
  title: string;
  message: string;
  timestampMs: number;
  /** Every unread failure-class or needs-human record of this session; opening the row marks all read. */
  recordIds: string[];
  primaryRecordId: string;
  /** False once the session is gone: the row stays until read or dismissed (C7). */
  sessionAvailable: boolean;
  /** An unread question record: the only row a Reply action may attach to (Story 5.6). */
  pendingQuestion: boolean;
}

export interface BackgroundActivityView {
  rows: BackgroundRow[];
  completedOkToday: number;
  running: number;
}

type UIType = NotificationData["notificationType"];

// The C7 join's own classification (the types the gate lets through for hidden sessions).
// Deliberately not a "pinned" set: pinning reads only the server's isPendingDecision.
function kindOf(type: UIType): BackgroundRowKind | null {
  switch (type) {
    case "error":
    case "task_failed":
      return "failure";
    case "approval_needed":
    case "question":
      return "needs_human";
    default:
      return null;
  }
}

function statusLabelFor(type: UIType): BackgroundStatusLabel {
  if (type === "question") return "NEEDS INPUT";
  if (type === "approval_needed") return "NEEDS APPROVAL";
  return "FAILED";
}

function startOfLocalDay(now: number): number {
  const d = new Date(now);
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}

interface Candidate {
  ref: HiddenSessionRef;
  available: boolean;
  summary?: HiddenSessionSummary;
}

/**
 * The C7 join: Background rows are a filtered view over notification history, not a
 * second source of truth. A hidden session contributes one row while history holds an
 * unread failure-class or needs-human record whose `sessionId` or `sessionName` equals
 * the session's id or title. A row leaves when its records are read, dismissed or
 * cleared, never because the session was deleted (`departed` lists hidden sessions seen
 * earlier that the latest list no longer returns). Routine completions are counted, never listed.
 */
export function selectBackgroundRows(
  history: readonly NotificationHistoryItem[],
  hiddenSessions: readonly HiddenSessionSummary[],
  departed: readonly HiddenSessionRef[] = [],
  now: number = Date.now(),
): BackgroundActivityView {
  const candidates: Candidate[] = [
    ...hiddenSessions.map((summary) => ({ ref: summary, available: true, summary })),
    ...departed
      .filter((d) => !hiddenSessions.some((s) => s.id === d.id || s.title === d.title))
      .map((ref) => ({ ref, available: false })),
  ];
  const byKey = new Map<string, Candidate>();
  for (const c of candidates) {
    byKey.set(c.ref.id, c);
    byKey.set(c.ref.title, c);
  }

  const dayStart = startOfLocalDay(now);
  const unreadBySession = new Map<Candidate, NotificationHistoryItem[]>();
  const failedToday = new Set<Candidate>();
  for (const record of history) {
    const kind = kindOf(record.notificationType);
    if (!kind) continue;
    const isFailure = kind === "failure";
    const candidate = byKey.get(record.sessionId) ?? byKey.get(record.sessionName);
    if (!candidate) continue;
    if (isFailure && record.timestamp >= dayStart) failedToday.add(candidate);
    if (record.isRead) continue;
    const list = unreadBySession.get(candidate) ?? [];
    list.push(record);
    unreadBySession.set(candidate, list);
  }

  const rows: BackgroundRow[] = [];
  for (const [candidate, records] of unreadBySession) {
    const failures = records.filter((r) => kindOf(r.notificationType) === "failure");
    const pool = failures.length > 0 ? failures : records;
    const primary = pool.reduce((a, b) => (b.timestamp > a.timestamp ? b : a));
    rows.push({
      key: `bg:${candidate.ref.id}`,
      sessionId: candidate.ref.id,
      sessionTitle: candidate.ref.title,
      kind: failures.length > 0 ? "failure" : "needs_human",
      statusLabel: statusLabelFor(primary.notificationType),
      title: primary.title || candidate.ref.title,
      message: primary.message,
      timestampMs: primary.timestamp,
      recordIds: records.map((r) => r.id),
      primaryRecordId: primary.id,
      sessionAvailable: candidate.available,
      pendingQuestion: replyAvailability(primary)?.kind === "replyable",
    });
  }
  rows.sort((a, b) => {
    if (a.kind !== b.kind) return a.kind === "failure" ? -1 : 1;
    return b.timestampMs - a.timestampMs;
  });

  let completedOkToday = 0;
  let running = 0;
  for (const s of hiddenSessions) {
    if (s.state === "running") running += 1;
    else if (s.state === "stopped" && s.updatedAtMs >= dayStart && !failedToday.has(byKey.get(s.id) as Candidate)) {
      completedOkToday += 1;
    }
  }
  return { rows, completedOkToday, running };
}
