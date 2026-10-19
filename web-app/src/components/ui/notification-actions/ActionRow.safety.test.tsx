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
