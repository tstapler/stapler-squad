import { mergeBackendHistory, recordToHistoryItem } from "@/lib/utils/notificationHistoryMerge";
import type { NotificationHistoryItem } from "@/lib/types/notification";
import type { NotificationHistoryRecord } from "@/gen/session/v1/session_pb";

const item = (overrides: Partial<NotificationHistoryItem> = {}): NotificationHistoryItem => ({
  id: "n1",
  sessionId: "s1",
  sessionName: "S",
  message: "m",
  timestamp: 0,
  notificationType: "warning",
  isRead: false,
  ...overrides,
});

describe("recordToHistoryItem", () => {
  it("carries the server-sent isPendingDecision onto the history item", () => {
    const record = {
      id: "r1",
      sessionId: "s1",
      sessionName: "S",
      title: "t",
      message: "m",
      priority: 2,
      notificationType: 5,
      isRead: false,
      isPendingDecision: true,
      occurrenceCount: 1,
      metadata: {},
    } as unknown as NotificationHistoryRecord;
    expect(recordToHistoryItem(record).isPendingDecision).toBe(true);
  });
});

describe("mergeBackendHistory", () => {
  it("live_toast_and_record_should_agree_by_construction: the server value replaces the stream value", () => {
    const streamAdded = item({ id: "notification-client-1", isPendingDecision: true });
    const fromServer = item({ id: "server-1", isPendingDecision: true, isRead: false });
    const merged = mergeBackendHistory([streamAdded], [fromServer]);
    expect(merged).toHaveLength(1);
    expect(merged[0].id).toBe("server-1");
    expect(merged[0].isPendingDecision).toBe(true);
  });

  it("an auto-remediated record read on the server is no longer pending locally", () => {
    const local = item({ id: "a", isPendingDecision: true });
    const server = item({ id: "a", isRead: true, isPendingDecision: false });
    expect(mergeBackendHistory([local], [server])[0].isPendingDecision).toBe(false);
  });

  it("keeps local callbacks on the server version", () => {
    const onView = jest.fn();
    const merged = mergeBackendHistory([item({ id: "a", onView })], [item({ id: "a" })]);
    expect(merged[0].onView).toBe(onView);
  });

  it("appends backend-only rows ahead of local ones and skips duplicate session+type", () => {
    const merged = mergeBackendHistory(
      [item({ id: "local", sessionId: "s1", notificationType: "error" })],
      [
        item({ id: "b1", sessionId: "s2", notificationType: "error" }),
        item({ id: "b2", sessionId: "s2", notificationType: "error" }),
      ],
    );
    expect(merged.map((n) => n.id)).toEqual(["b1", "local"]);
  });
});
