import { act, renderHook } from "@testing-library/react";
import { useStableRowOrder } from "../useStableRowOrder";
import type { NotificationHistoryItem } from "@/lib/types/notification";

const item = (id: string, timestamp: number): NotificationHistoryItem => ({
  id,
  sessionId: id,
  sessionName: id,
  message: "m",
  timestamp,
  isRead: false,
  notificationType: "info",
});

describe("useStableRowOrder", () => {
  it("passes history straight through when not holding", () => {
    const history = [item("a", 3), item("b", 2)];
    const { result } = renderHook(({ hold }) => useStableRowOrder(history, hold), { initialProps: { hold: false } });
    expect(result.current.items).toBe(history);
    expect(result.current.heldCount).toBe(0);
  });

  it("freezes a bumped row's recency and withholds new rows while held (TR-5)", () => {
    let history = [item("a", 3), item("b", 2)];
    const { result, rerender } = renderHook(({ hold }) => useStableRowOrder(history, hold), { initialProps: { hold: true } });
    rerender({ hold: true });

    history = [item("b", 10), item("a", 3), item("new", 11)]; // b was bumped, new arrived
    rerender({ hold: true });
    expect(result.current.heldCount).toBe(1);
    expect(result.current.items.map((n) => [n.id, n.timestamp])).toEqual([
      ["b", 2],
      ["a", 3],
    ]);

    act(() => result.current.release());
    expect(result.current.heldCount).toBe(0);
    expect(result.current.items).toBe(history);
  });

  it("applies the live order again once the hold ends", () => {
    let history = [item("a", 3)];
    const { result, rerender } = renderHook(({ hold }) => useStableRowOrder(history, hold), { initialProps: { hold: true } });
    rerender({ hold: true });
    history = [item("a", 3), item("n", 5)];
    rerender({ hold: false });
    expect(result.current.items).toBe(history);
  });
});
