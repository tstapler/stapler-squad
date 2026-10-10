import { act, renderHook } from "@testing-library/react";
import { toastQueueReducer, useToastQueue, type ToastQueue } from "@/lib/hooks/useToastQueue";
import { TOAST_STALE_MS, ACTIONABLE_TOAST_STALE_MS } from "@/lib/notification-policy";
import type { NotificationData } from "@/lib/types/notification";

function toast(overrides: Partial<NotificationData> = {}): NotificationData {
  return {
    id: "t1",
    sessionId: "s1",
    sessionName: "Session",
    message: "m",
    timestamp: 0,
    ...overrides,
  };
}

describe("toastQueueReducer", () => {
  it("add keeps one toast per session, newest wins", () => {
    const queue: ToastQueue = [toast({ id: "a", sessionId: "s1" }), toast({ id: "b", sessionId: "s2" })];
    const next = toastQueueReducer(queue, { type: "add", notification: toast({ id: "c", sessionId: "s1" }) });
    expect(next.map((n) => n.id)).toEqual(["b", "c"]);
  });

  it("add never displaces an approval toast with a non-approval one", () => {
    const queue: ToastQueue = [toast({ id: "a", onApprove: () => {} })];
    const next = toastQueueReducer(queue, { type: "add", notification: toast({ id: "b" }) });
    expect(next).toBe(queue);
  });

  it("append replaces a toast with the same action key and stacks otherwise", () => {
    const first = toast({ id: "a", sessionId: "", metadata: { actionToastKey: "k" } });
    const keyed = toastQueueReducer([first], {
      type: "append",
      notification: toast({ id: "b", sessionId: "", metadata: { actionToastKey: "k" } }),
      replaceKey: "k",
    });
    expect(keyed.map((n) => n.id)).toEqual(["b"]);

    const stacked = toastQueueReducer([first], {
      type: "append",
      notification: toast({ id: "c", sessionId: "" }),
    });
    expect(stacked.map((n) => n.id)).toEqual(["a", "c"]);
  });

  it("removes by id set, approval id and session ids, ignoring the empty session", () => {
    const queue: ToastQueue = [
      toast({ id: "a", sessionId: "s1", metadata: { approval_id: "ap1" } }),
      toast({ id: "b", sessionId: "s2" }),
      toast({ id: "c", sessionId: "" }),
    ];
    expect(toastQueueReducer(queue, { type: "remove", ids: new Set(["b"]) }).map((n) => n.id)).toEqual(["a", "c"]);
    expect(toastQueueReducer(queue, { type: "removeByApprovalId", approvalId: "ap1" }).map((n) => n.id)).toEqual(["b", "c"]);
    expect(
      toastQueueReducer(queue, { type: "removeBySessionIds", sessionIds: new Set(["s2", ""]) }).map((n) => n.id),
    ).toEqual(["a"]);
  });

  it("returns the same array when a remove matches nothing", () => {
    const queue: ToastQueue = [toast({ id: "a" })];
    expect(toastQueueReducer(queue, { type: "remove", ids: new Set(["zzz"]) })).toBe(queue);
  });

  it("prune applies the short window to plain toasts and the long one to approvals", () => {
    const queue: ToastQueue = [
      toast({ id: "info", notificationType: "info" }),
      toast({ id: "approval", notificationType: "approval_needed" }),
    ];
    const afterShort = toastQueueReducer(queue, { type: "prune", now: TOAST_STALE_MS });
    expect(afterShort.map((n) => n.id)).toEqual(["approval"]);
    const afterLong = toastQueueReducer(queue, { type: "prune", now: ACTIONABLE_TOAST_STALE_MS });
    expect(afterLong).toEqual([]);
  });

  it("prune leaves toasts the keep predicate exempts", () => {
    const queue: ToastQueue = [toast({ id: "a", notificationType: "info" })];
    const next = toastQueueReducer(queue, { type: "prune", now: ACTIONABLE_TOAST_STALE_MS * 10, keep: () => true });
    expect(next).toBe(queue);
  });
});

describe("useToastQueue", () => {
  it("keeps queueRef synchronously current with the rendered queue", () => {
    const { result } = renderHook(() => useToastQueue());
    act(() => {
      result.current.dispatch({ type: "add", notification: toast({ id: "a" }) });
      expect(result.current.queueRef.current.map((n) => n.id)).toEqual(["a"]);
    });
    expect(result.current.queue.map((n) => n.id)).toEqual(["a"]);
  });
});
