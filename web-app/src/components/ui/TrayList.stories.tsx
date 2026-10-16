// +feature: notification-tray
import type { Meta, StoryObj } from "@storybook/react";
import { createRef } from "react";
import type { NotificationHistoryItem } from "@/lib/types/notification";
import { flattenGroups } from "@/lib/utils/notificationGrouping";
import { TrayList } from "./TrayList";

const rows: NotificationHistoryItem[] = [
  { id: "p1", sessionId: "s1", sessionName: "api-server", message: "Approve rm -rf build?", timestamp: 3, isRead: false, notificationType: "approval_needed", isPendingDecision: true, metadata: { approval_id: "a1" } },
  { id: "i1", sessionId: "s2", sessionName: "web-app", message: "Task finished", timestamp: 2, isRead: false, notificationType: "task_complete" },
  { id: "i2", sessionId: "s3", sessionName: "docs", message: "Build passed", timestamp: 1, isRead: true, notificationType: "info" },
];

const { rows: flat, setSize } = flattenGroups(rows, new Set());

// The virtualized list: pinned "Needs attention" group first, then one group per session.
const meta: Meta<typeof TrayList> = {
  component: TrayList,
  title: "UI/TrayList",
  decorators: [(Story) => <div style={{ width: 400, height: 480, overflow: "auto" }}><Story /></div>],
  args: {
    rows: flat,
    setSize,
    itemProps: {
      resolvedApprovals: {},
      pendingApprovals: {},
      blockedApprovals: {},
      failedApprovals: {},
      resolveApproval: () => {},
      handleNotificationClick: () => {},
    },
    scrollRef: createRef<HTMLDivElement>(),
    hasMore: false,
    loading: false,
    onLoadMore: () => {},
    onToggleGroup: () => {},
    onDismissGroup: () => {},
    onDismissSession: () => {},
    groupForId: () => undefined,
  },
};
export default meta;
type Story = StoryObj<typeof TrayList>;

export const Default: Story = {};
export const Offline: Story = { args: { offlineReason: "Offline" } };
