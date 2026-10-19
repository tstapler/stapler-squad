import { replyAvailability } from "@/lib/reply/replyQuestion";
import type { NotificationData, NotificationHistoryItem } from "@/lib/types/notification";

/** A hidden session as the Background section needs it. `id` is the title today (types.proto:11). */
export interface HiddenSessionRef {
  id: string;
  title: string;
}

export type BackgroundOutcome = "completed" | "failed" | "unknown";

export interface HiddenSessionSummary extends HiddenSessionRef {
  state: "running" | "stopped" | "other";
  updatedAtMs: number;
  /** Server-computed outcome; the hook always sets it ("unknown" when an older server omits it). */
  backgroundOutcome?: BackgroundOutcome;
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

// ---------------------------------------------------------------------------
// Activity feed (Epic 1): one row per hidden session, exceptions pinned first.
// ---------------------------------------------------------------------------

export type SessionRole = "review" | "diagnose" | "triage" | "other";
export type ActivityState =
  | "running"
  | "finished"
  | "stopped_unknown"
  | "exception_failure"
  | "exception_needs_human"
  | "departed";

export interface ActivityRow {
  key: string;
  sessionId: string;
  sessionTitle: string;
  role: SessionRole;
  state: ActivityState;
  timestampMs: number;
  /** Set for the two exception states: the unread record(s) behind the row. */
  exception?: BackgroundRow;
}

export type RollupVariant = "none" | "no_failures_reported" | "needs_attention";

export interface BackgroundRollup {
  variant: RollupVariant;
  total: number;
  /** Rows in an exception state; the only count the tab's accessible name carries. */
  exceptions: number;
  lastActivityMs: number | null;
}

export function isExceptionState(state: ActivityState): boolean {
  return state === "exception_failure" || state === "exception_needs_human";
}

export function roleOf(session: HiddenSessionRef): SessionRole {
  const prefix = (session.title || session.id).split(":", 1)[0];
  return prefix === "review" || prefix === "diagnose" || prefix === "triage" ? prefix : "other";
}

// `finished` needs positive server evidence; history never supplies it (ADR-001 decision 6).
function stateOfSession(s: HiddenSessionSummary): ActivityState {
  if (s.state === "running") return "running";
  if (s.backgroundOutcome === "failed") return "exception_failure";
  if (s.state === "stopped" && s.backgroundOutcome === "completed") return "finished";
  return "stopped_unknown";
}

export interface BuildActivityOptions {
  /** Rows of the previous poll: a session missing now becomes `departed` for one cycle. */
  previous?: readonly ActivityRow[];
  /** Hidden sessions seen earlier that the list no longer returns (keeps their unread exceptions). */
  departed?: readonly HiddenSessionRef[];
  now?: number;
  /** Flag `background_activity_feed` off: emit only the exception rows (the pre-feed behaviour). */
  exceptionsOnly?: boolean;
}

/**
 * Exceptions (unread failure/needs-human records, or a server-reported failed outcome) pin
 * first and win over the session's own row; the rest follow newest-first. Pure and
 * deterministic, so the same input polled twice yields the same order.
 */
export function buildActivityRows(
  history: readonly NotificationHistoryItem[],
  hiddenSessions: readonly HiddenSessionSummary[],
  { previous = [], departed = [], now = Date.now(), exceptionsOnly = false }: BuildActivityOptions = {},
): ActivityRow[] {
  const exceptions = selectBackgroundRows(history, hiddenSessions, departed, now).rows;
  const exceptionBySession = new Map(exceptions.map((r) => [r.sessionId, r]));

  const pinned: ActivityRow[] = exceptions.map((r) => ({
    key: r.key,
    sessionId: r.sessionId,
    sessionTitle: r.sessionTitle,
    role: roleOf({ id: r.sessionId, title: r.sessionTitle }),
    state: r.kind === "failure" ? "exception_failure" : "exception_needs_human",
    timestampMs: r.timestampMs,
    exception: r,
  }));

  if (exceptionsOnly) return pinned;

  const rest: ActivityRow[] = [];
  for (const s of hiddenSessions) {
    if (exceptionBySession.has(s.id)) continue;
    rest.push({
      key: `bg:${s.id}`,
      sessionId: s.id,
      sessionTitle: s.title,
      role: roleOf(s),
      state: stateOfSession(s),
      timestampMs: s.updatedAtMs,
    });
  }
  rest.sort((a, b) => {
    const aFailed = a.state === "exception_failure";
    const bFailed = b.state === "exception_failure";
    if (aFailed !== bFailed) return aFailed ? -1 : 1;
    return b.timestampMs - a.timestampMs || a.sessionId.localeCompare(b.sessionId);
  });

  const emitted = new Set([...pinned, ...rest].map((r) => r.key));
  const gone: ActivityRow[] = previous
    .filter((p) => p.state !== "departed" && !emitted.has(p.key))
    .map((p) => ({ ...p, state: "departed" as const, exception: undefined }));

  return [...pinned, ...rest, ...gone];
}

export function buildRollup(rows: readonly ActivityRow[]): BackgroundRollup {
  const live = rows.filter((r) => r.state !== "departed");
  const exceptions = live.filter((r) => isExceptionState(r.state)).length;
  const times = live.map((r) => r.timestampMs).filter((t) => t > 0);
  return {
    variant: live.length === 0 ? "none" : exceptions > 0 ? "needs_attention" : "no_failures_reported",
    total: live.length,
    exceptions,
    lastActivityMs: times.length > 0 ? Math.max(...times) : null,
  };
}

/** Roll-up line; never claims health, since a stopped session with no failure record is not proof. */
export function rollupCopy(rollup: BackgroundRollup, relativeTime: (ms: number) => string): string {
  if (rollup.variant === "none") {
    return "No background work. Review and diagnose sessions will appear here while they run.";
  }
  const noun = rollup.total === 1 ? "background session" : "background sessions";
  const head =
    rollup.variant === "needs_attention"
      ? `${rollup.total} ${noun}, ${rollup.exceptions} ${rollup.exceptions === 1 ? "needs" : "need"} attention.`
      : `${rollup.total} ${noun}, no failures reported.`;
  return rollup.lastActivityMs === null ? head : `${head} Last activity ${relativeTime(rollup.lastActivityMs)}.`;
}
