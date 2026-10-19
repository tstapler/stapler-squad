import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ActionOutcome } from "@/gen/session/v1/notification_action_pb";
import { NotificationItem, type NotificationItemProps } from "./NotificationItem";
import { makeApprovalDescriptors, makeDecisionDescriptors, makeDescriptor, makeNotification } from "./__fixtures__/notificationFixtures";
import { invokeMock, respondWith } from "./__fixtures__/actionRowHarness";

jest.mock("next/link", () => ({
  __esModule: true,
  default: ({ href, children, ...props }: { href: string; children: React.ReactNode }) => (
    <a href={href} {...props}>
      {children}
    </a>
  ),
}));
jest.mock("@/lib/api/notificationActionApi", () => ({ invokeNotificationAction: jest.fn() }));

let inlineFlag = true;
jest.mock("@/lib/hooks/useFeatureFlagValue", () => ({
  ...jest.requireActual("@/lib/hooks/useFeatureFlagValue"),
  useFeatureFlagValue: (_name: string, _scope: string | undefined, _fallback: boolean, enabled = true) => inlineFlag && enabled,
}));

const approval = (over = {}) =>
  makeNotification({
    notificationType: "approval_needed",
    metadata: { approval_id: "appr-1", tool_name: "Bash", tool_input_command: "git push origin main" },
    actions: makeApprovalDescriptors(),
    ...over,
  });

function renderItem(n = approval(), props: Partial<NotificationItemProps> = {}) {
  const resolveApproval = jest.fn();
  const utils = render(
    <NotificationItem
      group={{ notification: n, count: 1, allIds: [n.id] }}
      resolvedApprovals={{}}
      pendingApprovals={{}}
      blockedApprovals={{}}
      failedApprovals={{}}
      resolveApproval={resolveApproval}
      handleNotificationClick={jest.fn()}
      {...props}
    />,
  );
  return { ...utils, resolveApproval };
}

beforeEach(() => {
  inlineFlag = true;
  invokeMock.mockReset();
});

describe("NotificationItem inline actions", () => {
  it("renders Approve and Deny from the descriptors with no legacy handler involved", async () => {
    respondWith(ActionOutcome.APPLIED);
    const onActionApplied = jest.fn();
    const { resolveApproval } = renderItem(approval(), { onActionApplied });
    expect(screen.getByTestId("action-approve")).toBeInTheDocument();
    expect(screen.getByTestId("action-deny")).toBeInTheDocument();
    expect(screen.queryByText("✓ Approve")).toBeNull();
    await userEvent.click(screen.getByTestId("action-approve"));
    await screen.findByText("Approved");
    expect(resolveApproval).not.toHaveBeenCalled();
    expect(onActionApplied).toHaveBeenCalledWith(["notif-1"]);
  });

  it("shows the command once, inside the action row", () => {
    renderItem();
    expect(screen.getAllByText("git push origin main")).toHaveLength(1);
    expect(screen.getByTestId("approval-command")).toHaveTextContent("git push origin main");
  });

  it("keeps the legacy Approve/Deny path when the flag is off", async () => {
    inlineFlag = false;
    const { resolveApproval } = renderItem();
    expect(screen.queryByTestId("action-row")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "✓ Approve" }));
    expect(resolveApproval).toHaveBeenCalledWith("appr-1", "allow", ["notif-1"]);
  });

  it("keeps the legacy path for an item whose descriptors are only Open", () => {
    renderItem(approval({ actions: [makeDescriptor()] }));
    expect(screen.queryByTestId("action-row")).toBeNull();
    expect(screen.getByRole("button", { name: "✓ Approve" })).toBeInTheDocument();
  });

  it("keeps the resolved badge over the action row for an approval already decided", () => {
    renderItem(approval(), { resolvedApprovals: { "appr-1": "allow" } });
    expect(screen.queryByTestId("action-row")).toBeNull();
    expect(screen.getByText("✓ Approved")).toBeInTheDocument();
  });

  it("uses the item's own link as the row's Open control", () => {
    renderItem();
    expect(screen.getByTestId("notification-view-session")).toBeInTheDocument();
    expect(screen.queryByTestId("action-open")).toBeNull();
  });

  it("renders a backlog decision with its choices and an Open to the backlog item", async () => {
    const n = makeNotification({
      notificationType: "question",
      metadata: { item_id: "item-1" },
      actions: makeDecisionDescriptors(),
    });
    renderItem(n);
    expect(screen.getByTestId("action-backlog_decision-accept")).toHaveTextContent("Mark done");
    expect(screen.getByTestId("notification-view-backlog")).toBeInTheDocument();
  });

  it("refetches descriptors through onRefreshActions on INVALID", async () => {
    respondWith(ActionOutcome.INVALID);
    const onRefreshActions = jest.fn();
    renderItem(approval(), { onRefreshActions });
    await userEvent.click(screen.getByTestId("action-approve"));
    await userEvent.click(await screen.findByTestId("action-refresh"));
    await waitFor(() => expect(onRefreshActions).toHaveBeenCalledTimes(1));
  });

  it("renders no dismiss control for an unread actionable item when removeFromHistory is omitted", () => {
    renderItem();
    expect(screen.queryByLabelText("Remove notification")).toBeNull();
  });
});
