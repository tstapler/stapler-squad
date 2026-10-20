import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { NotificationCommandsContext, type NotificationCommandsValue } from "@/lib/contexts/notificationContexts";
import { NotificationItem } from "./NotificationItem";
import { makeDescriptor, makeGuidanceRow, makeNotification, makeQuestionDescriptors } from "./__fixtures__/notificationFixtures";
import type { PlainGuidanceRequest } from "@/lib/api/guidanceApi";

jest.mock("@/lib/nav", () =>
  require("@/lib/nav/testing-mock").navMockFromNext({
    link: () => ({
      __esModule: true,
      default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => (
        <a href={href} {...props}>
          {children}
        </a>
      ),
    }),
  }),
);
jest.mock("@/lib/api/notificationActionApi", () => ({ invokeNotificationAction: jest.fn() }));
jest.mock("@/lib/hooks/useFeatureFlagValue", () => ({
  ...jest.requireActual("@/lib/hooks/useFeatureFlagValue"),
  useFeatureFlagValue: (_n: string, _s: string | undefined, _f: boolean, enabled = true) => enabled,
}));
let row: PlainGuidanceRequest | undefined;
jest.mock("@/lib/hooks/useGuidanceRow", () => ({ useGuidanceRow: (id?: string) => (id ? row : undefined) }));

const question = (over = {}) =>
  makeNotification({
    id: "n-q",
    notificationType: "question",
    isRead: false,
    isPendingDecision: true,
    metadata: { guidance_request_id: "q1", question_id: "q1" },
    actions: [makeDescriptor()],
    ...over,
  });

function renderQuestion(n = question(), props: Record<string, unknown> = {}) {
  const markAnswerHandled = jest.fn();
  const commands = { markAnswerHandled } as unknown as NotificationCommandsValue;
  render(
    <NotificationCommandsContext.Provider value={commands}>
      <NotificationItem
        group={{ notification: n, count: 1, allIds: [n.id] }}
        resolvedApprovals={{}}
        pendingApprovals={{}}
        blockedApprovals={{}}
        failedApprovals={{}}
        resolveApproval={jest.fn()}
        handleNotificationClick={jest.fn()}
        {...props}
      />
    </NotificationCommandsContext.Provider>,
  );
  return { markAnswerHandled };
}

beforeEach(() => {
  row = undefined;
});

describe("NotificationItem question card", () => {
  it("renders the shared question card from the durable row even when the descriptors are only Open", () => {
    row = makeGuidanceRow({ status: "answered", answer: "Redis", deliveryState: "sent", unacknowledged: true });
    renderQuestion();
    expect(screen.getByTestId("delivery-chip")).toHaveTextContent("Unconfirmed");
    expect(screen.getByTestId("delivery-text")).toHaveTextContent("Sent, but the agent hasn't confirmed yet. Open session.");
  });

  it("keeps the plain row when the question has no durable row", () => {
    renderQuestion();
    expect(screen.queryByTestId("delivery-line")).toBeNull();
    expect(screen.queryByTestId("action-row")).toBeNull();
  });

  it("marks the group handled in this browser only after the inline confirm", async () => {
    row = makeGuidanceRow({ status: "answered", answer: "Redis", deliveryState: "sent", unacknowledged: true });
    const { markAnswerHandled } = renderQuestion();
    await userEvent.click(screen.getByRole("button", { name: "Mark as handled" }));
    expect(markAnswerHandled).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Mark handled" }));
    expect(markAnswerHandled).toHaveBeenCalledWith(["n-q"]);
  });

  it("asks for a history refresh when an Unconfirmed answer is still read, so the server's reopen comes back", () => {
    row = makeGuidanceRow({ status: "answered", answer: "Redis", deliveryState: "sent", unacknowledged: true });
    const onRefreshActions = jest.fn();
    renderQuestion(question({ isRead: true, isPendingDecision: false }), { onRefreshActions });
    expect(onRefreshActions).toHaveBeenCalledTimes(1);
  });

  it("does not refresh when the card is already in Needs you", () => {
    row = makeGuidanceRow({ status: "answered", answer: "Redis", deliveryState: "sent", unacknowledged: true });
    const onRefreshActions = jest.fn();
    renderQuestion(question(), { onRefreshActions });
    expect(onRefreshActions).not.toHaveBeenCalled();
  });

  it("refreshes once when a terminal answer retires a card the server still lists as unread", () => {
    row = makeGuidanceRow({ status: "cancelled", cancelReason: "answered_in_terminal" });
    const onRefreshActions = jest.fn();
    renderQuestion(question({ actions: makeQuestionDescriptors() }), { onRefreshActions });
    expect(screen.getByTestId("delivery-text")).toHaveTextContent("Already handled in the session. No action needed.");
    expect(onRefreshActions).toHaveBeenCalledTimes(1);
  });
});

describe("NotificationItem saved answers", () => {
  it("tells the Saved for later list to re-read as soon as its answer is saved", () => {
    row = makeGuidanceRow({ source: "agent_mcp", status: "answered", answer: "Redis", deliveryState: "saved_for_later", deliveryRoute: "resume" });
    const changed = jest.fn();
    window.addEventListener("ssq:saved-answers-changed", changed);
    renderQuestion(question({ isRead: true, isPendingDecision: false }));
    window.removeEventListener("ssq:saved-answers-changed", changed);
    expect(changed).toHaveBeenCalledTimes(1);
  });
});
