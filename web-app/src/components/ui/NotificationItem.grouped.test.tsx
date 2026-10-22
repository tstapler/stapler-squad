import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ActionOutcome } from "@/gen/session/v1/notification_action_pb";
import { NotificationItem } from "./NotificationItem";
import { makeApprovalDescriptors, makeNotification } from "./__fixtures__/notificationFixtures";
import { invokeMock, respondWith } from "./__fixtures__/actionRowHarness";

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
let inlineOn = true;
jest.mock("@/lib/hooks/useFeatureFlagValue", () => ({
  ...jest.requireActual("@/lib/hooks/useFeatureFlagValue"),
  useFeatureFlagValue: (_n: string, _s: string | undefined, _f: boolean, enabled = true) => inlineOn && enabled,
}));

// Twenty approvals of one session collapse into ONE history row (server_notifications/store.go), so the
// row addresses a single approval. Deciding it decides that one; the other nineteen are still pending.
const grouped = (count: number) =>
  makeNotification({
    id: "appr-20",
    notificationType: "approval_needed",
    metadata: { approval_id: "appr-20", tool_name: "Bash", tool_input_command: "make step-20" },
    actions: makeApprovalDescriptors("appr-20"),
    occurrenceCount: count,
  });

function renderGroup(count: number, props: Record<string, unknown> = {}) {
  const n = grouped(count);
  const resolveApproval = jest.fn();
  render(
    <NotificationItem
      group={{ notification: n, count, allIds: [n.id] }}
      resolvedApprovals={{}}
      pendingApprovals={{}}
      blockedApprovals={{}}
      failedApprovals={{}}
      resolveApproval={resolveApproval}
      handleNotificationClick={jest.fn()}
      {...props}
    />,
  );
  return { resolveApproval };
}

beforeEach(() => {
  invokeMock.mockReset();
  inlineOn = true;
});

describe("an xN approval row", () => {
  it("does not acknowledge the whole row from the legacy Approve button either", async () => {
    inlineOn = false;
    const { resolveApproval } = renderGroup(20);
    await userEvent.click(screen.getByRole("button", { name: /Approve/ }));
    expect(resolveApproval).toHaveBeenCalledWith("appr-20", "allow", []);
  });

  it("shows the count and says how many approvals still wait after one is approved, without marking the row resolved", async () => {
    respondWith(ActionOutcome.APPLIED);
    const onActionApplied = jest.fn();
    const onRefreshActions = jest.fn();
    renderGroup(20, { onActionApplied, onRefreshActions });
    expect(screen.getByLabelText("20 occurrences")).toHaveTextContent("x20");

    await userEvent.click(screen.getByTestId("action-approve"));
    await waitFor(() => expect(screen.getByTestId("action-status")).toBeInTheDocument(), { timeout: 5000 });

    expect(invokeMock).toHaveBeenCalledTimes(1);
    expect(invokeMock.mock.calls[0][0].targetKey).toBe("appr-20");
    expect(screen.getByTestId("action-status")).toHaveTextContent("Approved. 19 more waiting.");
    expect(onActionApplied).not.toHaveBeenCalled();
    expect(onRefreshActions).toHaveBeenCalledTimes(1);
  });

  it("marks a single approval row resolved as before", async () => {
    respondWith(ActionOutcome.APPLIED);
    const onActionApplied = jest.fn();
    renderGroup(1, { onActionApplied });
    await userEvent.click(screen.getByTestId("action-approve"));
    await waitFor(() => expect(onActionApplied).toHaveBeenCalledWith(["appr-20"], expect.any(String)), { timeout: 5000 });
    expect(screen.getByTestId("action-status")).toHaveTextContent(/^Approved$/);
  });
});
