import { act, fireEvent, screen } from "@testing-library/react";
import { ActionOutcome } from "@/gen/session/v1/notification_action_pb";
import { setTimingForTests } from "@/lib/hooks/interactionTiming";
import { makeApprovalDescriptors, makeDecisionDescriptors } from "../__fixtures__/notificationFixtures";
import { invokeMock, renderRow, respondWith } from "../__fixtures__/actionRowHarness";

jest.mock("@/lib/api/notificationActionApi", () => ({ invokeNotificationAction: jest.fn() }));

const advance = (ms: number) => act(() => void jest.advanceTimersByTime(ms));
const click = (testId: string) => act(() => void fireEvent.click(screen.getByTestId(testId)));

beforeEach(() => {
  jest.useFakeTimers();
  invokeMock.mockReset();
  respondWith(ActionOutcome.APPLIED);
  window.sessionStorage.clear();
});
afterEach(() => {
  jest.useRealTimers();
  setTimingForTests({ settleMs: 0, commitDelayMs: 0 });
});

describe("Story 2.10b: settled layout (UX-AC62)", () => {
  it("ignores a control activated 100 ms after it mounted and invokes on the activation at 400 ms", async () => {
    setTimingForTests({ settleMs: 300, commitDelayMs: 0 });
    renderRow({ descriptors: makeApprovalDescriptors() });
    advance(100);
    click("action-deny");
    expect(screen.queryByTestId("deny-step")).toBeNull();
    advance(300);
    click("action-deny");
    expect(screen.getByTestId("deny-step")).toBeInTheDocument();
  });

  it("sends once for Mark done activated before and after the window", async () => {
    setTimingForTests({ settleMs: 300, commitDelayMs: 0 });
    renderRow({ descriptors: makeDecisionDescriptors() });
    advance(100);
    click("action-backlog_decision-accept");
    expect(invokeMock).not.toHaveBeenCalled();
    advance(300);
    await act(async () => void fireEvent.click(screen.getByTestId("action-backlog_decision-accept")));
    expect(invokeMock).toHaveBeenCalledTimes(1);
  });
});

const DELAY = 3_000;
const rework = "action-backlog_decision-rework";
const accept = "action-backlog_decision-accept";
const pathname = { current: "/" };
jest.mock("next/navigation", () => ({ usePathname: () => pathname.current }));

describe("Task 2.10c: commit delay with Undo (UX-AC63)", () => {
  beforeEach(() => {
    setTimingForTests({ settleMs: 0, commitDelayMs: DELAY });
    pathname.current = "/";
  });

  it("holds Approve: the button is aria-disabled, focus is on Undo, Undo sends nothing and restores the row", () => {
    const announce = jest.fn();
    renderRow({ descriptors: makeApprovalDescriptors() }, { announce });
    click("action-approve");
    expect(screen.getByTestId("action-approve")).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByTestId("action-approve")).toHaveTextContent("Approving in 3s");
    expect(document.activeElement).toBe(screen.getByTestId("action-undo"));
    advance(2_000);
    click("action-undo");
    advance(DELAY * 2);
    expect(invokeMock).not.toHaveBeenCalled();
    expect(screen.getByTestId("action-approve")).not.toHaveAttribute("aria-disabled");
    expect(screen.getByTestId("action-approve")).toHaveTextContent("Approve");
    expect(screen.queryByTestId("hold-cancelled")).toBeNull();
    expect(announce.mock.calls.map(([t]) => t)).toEqual(["Approval pending", "Approval undone"]);
  });

  it("sends exactly one call after 3 s with the row open, and counts down as plain text", async () => {
    renderRow({ descriptors: makeApprovalDescriptors() });
    click("action-approve");
    advance(1_000);
    expect(screen.getByTestId("action-approve")).toHaveTextContent("Approving in 2s");
    expect(screen.getByTestId("action-row").querySelector("[aria-live], [role=status], [role=alert]")).toBeNull();
    advance(1_999);
    expect(invokeMock).not.toHaveBeenCalled();
    await act(async () => void jest.advanceTimersByTime(1));
    expect(invokeMock).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["Send back", rework],
    ["Mark done", accept],
  ])("holds %s too", async (_label, testId) => {
    renderRow({ descriptors: makeDecisionDescriptors() });
    click(testId);
    expect(invokeMock).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(screen.getByTestId("action-undo"));
    await act(async () => void jest.advanceTimersByTime(DELAY));
    expect(invokeMock).toHaveBeenCalledTimes(1);
  });

  it("sends Deny at once, with no hold", async () => {
    renderRow({ descriptors: makeApprovalDescriptors() });
    click("action-deny");
    await act(async () => void fireEvent.click(screen.getByTestId("action-send-denial")));
    expect(invokeMock).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId("action-undo")).toBeNull();
  });

  const leaves: Array<[string, () => void]> = [
    ["visibilitychange to hidden", () => {
      Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "hidden" });
      document.dispatchEvent(new Event("visibilitychange"));
      Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "visible" });
    }],
    ["pagehide", () => window.dispatchEvent(new Event("pagehide"))],
    ["offline", () => window.dispatchEvent(new Event("offline"))],
    ["Back (popstate)", () => window.dispatchEvent(new PopStateEvent("popstate"))],
    ["a click outside the tray", () => document.body.dispatchEvent(new MouseEvent("pointerdown", { bubbles: true }))],
  ];
  it.each(leaves)("cancels, never commits, on %s, and restores the card with the cancellation line", (_name, leave) => {
    const announce = jest.fn();
    const { unmount, rowId } = renderRow({ descriptors: makeApprovalDescriptors() }, { announce });
    click("action-approve");
    advance(1_000);
    act(leave);
    advance(DELAY * 3);
    expect(invokeMock).not.toHaveBeenCalled();
    expect(screen.getByTestId("hold-cancelled")).toHaveTextContent("Approval cancelled when you left. Tap Approve to try again.");
    expect(screen.getByTestId("action-approve")).not.toHaveAttribute("aria-disabled");
    expect(announce.mock.calls.filter(([t]) => t === "Approval undone")).toHaveLength(1);
    // The flag survives a discarded page: a fresh mount of the same row shows it again.
    unmount();
    renderRow({ descriptors: makeApprovalDescriptors(), rowId });
    expect(screen.getByTestId("hold-cancelled")).toBeInTheDocument();
  });

  it("cancels on a route change", () => {
    const { refreshRow } = renderRow({ descriptors: makeApprovalDescriptors() });
    click("action-approve");
    pathname.current = "/sessions";
    refreshRow();
    advance(DELAY * 2);
    expect(invokeMock).not.toHaveBeenCalled();
  });

  it("clears the cancellation line when the operator taps Approve again", () => {
    renderRow({ descriptors: makeApprovalDescriptors() });
    click("action-approve");
    act(() => void window.dispatchEvent(new Event("pagehide")));
    click("action-approve");
    expect(screen.queryByTestId("hold-cancelled")).toBeNull();
  });
});
