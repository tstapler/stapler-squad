import { selectBackgroundRows, type HiddenSessionSummary } from "@/lib/utils/backgroundActivity";
import type { NotificationHistoryItem } from "@/lib/types/notification";

const NOW = new Date(2026, 9, 9, 14, 0, 0).getTime();
const MIN = 60_000;

function rec(over: Partial<NotificationHistoryItem> = {}): NotificationHistoryItem {
  return {
    id: "n1",
    sessionId: "review:h1",
    sessionName: "review:h1",
    message: "tests failed",
    timestamp: NOW - 4 * MIN,
    notificationType: "error",
    isRead: false,
    ...over,
  };
}

function sess(over: Partial<HiddenSessionSummary> = {}): HiddenSessionSummary {
  return { id: "review:h1", title: "review:h1", state: "running", updatedAtMs: NOW - MIN, ...over };
}

describe("selectBackgroundRows (C7 join)", () => {
  it("selectBackgroundRows_should_join_unread_failure_class_records_to_hidden_sessions_when_ids_or_titles_match", () => {
    const sessions = [sess({ id: "uuid-1", title: "review:h1" }), sess({ id: "uuid-2", title: "triage:h2" })];
    const history = [
      rec({ id: "a", sessionId: "uuid-1", sessionName: "something-else" }), // by id
      rec({ id: "b", sessionId: "other", sessionName: "triage:h2", notificationType: "question" }), // by title
      rec({ id: "c", sessionId: "visible-1", sessionName: "visible-1" }), // not hidden
    ];
    const { rows } = selectBackgroundRows(history, sessions, [], NOW);
    expect(rows.map((r) => r.primaryRecordId)).toEqual(["a", "b"]);
    expect(rows[0]).toMatchObject({ kind: "failure", statusLabel: "FAILED", sessionAvailable: true });
    expect(rows[1]).toMatchObject({ kind: "needs_human", statusLabel: "NEEDS INPUT" });
  });

  it("covers every failure-class and needs-human type and no other", () => {
    const sessions = [sess()];
    const types = ["error", "task_failed", "approval_needed", "question"] as const;
    for (const t of types) {
      expect(selectBackgroundRows([rec({ notificationType: t })], sessions, [], NOW).rows).toHaveLength(1);
    }
    for (const t of ["warning", "info", "task_complete", "progress", "auto_approved", "system"] as const) {
      expect(selectBackgroundRows([rec({ notificationType: t })], sessions, [], NOW).rows).toHaveLength(0);
    }
  });

  it("orders failures before needs-human, newest first within each, one row per session", () => {
    const sessions = [sess({ id: "s1", title: "s1" }), sess({ id: "s2", title: "s2" }), sess({ id: "s3", title: "s3" })];
    const history = [
      rec({ id: "q", sessionId: "s1", sessionName: "s1", notificationType: "question", timestamp: NOW - MIN }),
      rec({ id: "f-old", sessionId: "s2", sessionName: "s2", timestamp: NOW - 30 * MIN }),
      rec({ id: "f-new", sessionId: "s3", sessionName: "s3", timestamp: NOW - 5 * MIN }),
      rec({ id: "f-new-2", sessionId: "s3", sessionName: "s3", timestamp: NOW - 6 * MIN }),
    ];
    const { rows } = selectBackgroundRows(history, sessions, [], NOW);
    expect(rows.map((r) => r.sessionId)).toEqual(["s3", "s2", "s1"]);
    expect(rows[0].recordIds.sort()).toEqual(["f-new", "f-new-2"]);
    expect(rows[0].primaryRecordId).toBe("f-new");
  });

  it("selectBackgroundRows_should_drop_row_when_read_or_dismissed_but_keep_no_longer_available_row_when_session_deleted", () => {
    const sessions = [sess()];
    const unread = rec();
    expect(selectBackgroundRows([unread], sessions, [], NOW).rows).toHaveLength(1);
    // read leaves
    expect(selectBackgroundRows([{ ...unread, isRead: true }], sessions, [], NOW).rows).toHaveLength(0);
    // dismissed or cleared: absent from history
    expect(selectBackgroundRows([], sessions, [], NOW).rows).toHaveLength(0);
    // deleted while unread: the row stays, marked unavailable
    const kept = selectBackgroundRows([unread], [], [{ id: "review:h1", title: "review:h1" }], NOW).rows;
    expect(kept).toHaveLength(1);
    expect(kept[0]).toMatchObject({ sessionAvailable: false, message: "tests failed" });
    // and leaves once read
    expect(selectBackgroundRows([{ ...unread, isRead: true }], [], [{ id: "review:h1", title: "review:h1" }], NOW).rows).toHaveLength(0);
  });

  it("selectBackgroundRows_should_emit_no_row_for_routine_completion_and_count_it_in_summary_line", () => {
    const sessions = [
      sess({ id: "ok1", title: "ok1", state: "stopped", updatedAtMs: NOW - 60 * MIN }),
      sess({ id: "ok2", title: "ok2", state: "stopped", updatedAtMs: NOW - 120 * MIN }),
      sess({ id: "old", title: "old", state: "stopped", updatedAtMs: new Date(2026, 9, 8, 23, 0).getTime() }),
      sess({ id: "failed", title: "failed", state: "stopped", updatedAtMs: NOW - 10 * MIN }),
      sess({ id: "run", title: "run", state: "running" }),
    ];
    const history = [
      rec({ id: "x", sessionId: "failed", sessionName: "failed", timestamp: NOW - 10 * MIN }),
      rec({ id: "done", sessionId: "ok1", sessionName: "ok1", notificationType: "task_complete" }),
    ];
    const view = selectBackgroundRows(history, sessions, [], NOW);
    expect(view.rows.map((r) => r.sessionId)).toEqual(["failed"]);
    expect(view.completedOkToday).toBe(2);
    expect(view.running).toBe(1);
  });

  it("a read failure record today still excludes its session from the completed-OK count", () => {
    const sessions = [sess({ id: "failed", title: "failed", state: "stopped", updatedAtMs: NOW - MIN })];
    const history = [rec({ sessionId: "failed", sessionName: "failed", isRead: true })];
    const view = selectBackgroundRows(history, sessions, [], NOW);
    expect(view.rows).toHaveLength(0);
    expect(view.completedOkToday).toBe(0);
  });

  it("flags only an unread question record as a pending question", () => {
    const sessions = [sess()];
    const q = selectBackgroundRows([rec({ notificationType: "question" })], sessions, [], NOW).rows[0];
    expect(q.pendingQuestion).toBe(true);
    const a = selectBackgroundRows([rec({ notificationType: "approval_needed" })], sessions, [], NOW).rows[0];
    expect(a.pendingQuestion).toBe(false);
  });
});
