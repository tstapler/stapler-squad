import { flattenGroups, groupBySession, selectPinnedDecisions, countNeedsAttention, NEEDS_ATTENTION_KEY } from "../notificationGrouping";
import { groupNotifications } from "../notificationGrouping";
import type { NotificationHistoryItem } from "@/lib/types/notification";

let n = 0;
function row(overrides: Partial<NotificationHistoryItem> = {}): NotificationHistoryItem {
  n += 1;
  return {
    id: `n-${n}`,
    sessionId: "s1",
    sessionName: "session one",
    message: "m",
    timestamp: 1_000 + n,
    isRead: false,
    notificationType: "info",
    ...overrides,
  };
}

describe("tray model", () => {
  beforeEach(() => {
    n = 0;
  });

  it("pins every isPendingDecision group first, keeps it out of session groups and ignores type", () => {
    const rows = [
      row({ id: "a", sessionId: "s1", notificationType: "task_complete" }),
      row({ id: "w", sessionId: "s2", notificationType: "warning", isPendingDecision: true }),
      // An auto-remediating WARNING has the same type but is not pending.
      row({ id: "aw", sessionId: "s3", notificationType: "warning", isPendingDecision: false }),
    ];
    const { rows: flat, needsAttention } = flattenGroups(rows, new Set());
    expect(needsAttention).toBe(1);
    expect(flat[0]).toMatchObject({ kind: "header", key: NEEDS_ATTENTION_KEY, count: 1, pinned: true });
    expect(flat[1]).toMatchObject({ kind: "group", pinned: true });
    const sessionGroupIds = flat.filter((r) => r.kind === "group" && !r.pinned).map((r) => (r as { key: string }).key);
    expect(sessionGroupIds.sort()).toEqual(["a", "aw"]);
    expect(sessionGroupIds).not.toContain("w");
  });

  it("shows the one-line note when rows exist but none need attention", () => {
    const { rows, needsAttention } = flattenGroups([row()], new Set());
    expect(needsAttention).toBe(0);
    expect(rows[0]).toMatchObject({ kind: "note", text: "Nothing needs attention" });
  });

  it("skips the rows of a collapsed session but keeps its header", () => {
    const rows = [row({ sessionId: "s1" }), row({ sessionId: "s1", notificationType: "error" }), row({ sessionId: "s2" })];
    const { rows: flat } = flattenGroups(rows, new Set(["s1"]));
    const headers = flat.filter((r) => r.kind === "header");
    expect(headers).toHaveLength(2);
    expect(flat.filter((r) => r.kind === "group")).toHaveLength(1);
    expect(headers.find((h) => h.key === "s1")).toMatchObject({ collapsed: true, count: 2 });
  });

  it("numbers selectable rows 1..N for aria-posinset", () => {
    const { rows, setSize } = flattenGroups([row({ sessionId: "s1" }), row({ sessionId: "s2" }), row({ sessionId: "s3" })], new Set());
    const positions = rows.filter((r) => r.kind === "group").map((r) => (r as { posInSet: number }).posInSet);
    expect(positions).toEqual([1, 2, 3]);
    expect(setSize).toBe(3);
  });

  it("selectPinnedDecisions and countNeedsAttention agree", () => {
    const rows = [row({ isPendingDecision: true, sessionId: "a" }), row({ sessionId: "b" })];
    expect(selectPinnedDecisions(groupNotifications(rows))).toHaveLength(1);
    expect(countNeedsAttention(rows)).toBe(1);
  });

  it("groupBySession counts records per session", () => {
    const sessions = groupBySession(groupNotifications([row({ sessionId: "s1" }), row({ sessionId: "s1", notificationType: "error" })]));
    expect(sessions).toHaveLength(1);
    expect(sessions[0].rowCount).toBe(2);
  });
});
