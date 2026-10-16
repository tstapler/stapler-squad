// +feature: ui-notification-item
import type { Meta, StoryObj } from "@storybook/react";
import type { NotificationHistoryItem } from "@/lib/types/notification";
import type { GroupedNotification } from "@/lib/utils/notificationGrouping";
import { NotificationItem } from "./NotificationItem";

// One notification card; approval state is injected via the Record props.
const meta: Meta<typeof NotificationItem> = {
  component: NotificationItem,
  title: "UI/NotificationItem",
  decorators: [(Story) => <div style={{ width: 420 }}><Story /></div>],
  args: {
    resolvedApprovals: {},
    pendingApprovals: {},
    blockedApprovals: {},
    failedApprovals: {},
    resolveApproval: () => {},
    removeFromHistory: () => {},
    handleNotificationClick: () => {},
  },
};
export default meta;
type Story = StoryObj<typeof NotificationItem>;

function make(overrides: Partial<NotificationHistoryItem> = {}, count = 1): GroupedNotification {
  const notification: NotificationHistoryItem = {
    id: "notif-1",
    sessionId: "sess-1",
    sessionName: "my-session",
    message: "Task finished successfully.",
    timestamp: Date.now() - 90_000,
    notificationType: "task_complete",
    isRead: false,
    ...overrides,
  };
  return { notification, count, allIds: [notification.id] };
}

const approval = (extra: Record<string, string> = {}) =>
  make({
    notificationType: "approval_needed",
    message: "Claude wants to run a command",
    metadata: { approval_id: "appr-1", tool_name: "Bash", tool_input_command: "git push --force origin main", cwd: "/Users/you/code/repo", ...extra },
  });

export const Info: Story = { args: { group: make({ notificationType: "info", message: "Session started." }) } };
export const TaskComplete: Story = { args: { group: make() } };
export const Read: Story = { args: { group: make({ isRead: true }) } };
export const Grouped: Story = { args: { group: make({}, 5) } };
export const Warning: Story = { args: { group: make({ notificationType: "warning", message: "Disk space is low." }) } };
export const Error: Story = { args: { group: make({ notificationType: "error", priority: "urgent", message: "Session crashed unexpectedly." }) } };
export const ApprovalNeeded: Story = { args: { group: approval() } };
export const ApprovalPending: Story = { args: { group: approval(), pendingApprovals: { "appr-1": true } } };
export const ApprovalApproved: Story = { args: { group: approval(), resolvedApprovals: { "appr-1": "allow" } } };
export const ApprovalDenied: Story = { args: { group: approval(), resolvedApprovals: { "appr-1": "deny" } } };
export const ApprovalExpired: Story = { args: { group: approval(), resolvedApprovals: { "appr-1": "expired" } } };
export const ApprovalAutoResolved: Story = {
  args: {
    group: approval({ reconciled: "true", classifier_rule_name: "Auto-allow safe git status checks" }),
    resolvedApprovals: { "appr-1": "allow" },
  },
};
export const ApprovalBlockedByCI: Story = {
  args: {
    group: approval(),
    blockedApprovals: { "appr-1": "Approval blocked: CI is failing on this branch. https://github.com/org/repo/pull/1/checks" },
  },
};
export const ApprovalFailed: Story = {
  args: { group: approval(), failedApprovals: { "appr-1": "Couldn't record your decision — try again." } },
};
export const BacklogLinked: Story = { args: { group: make({ metadata: { item_id: "item-1" }, message: "Backlog item completed." }) } };
export const LongContent: Story = {
  args: {
    group: approval({
      tool_input_command: "find / -name '*.log' -type f -mtime +30 -exec rm -f {} \; && echo " + "very-long-argument ".repeat(12),
      cwd: "/Users/you/code/github.com/some-organization/some-deeply-nested-repository/packages/service",
    }),
  },
};
