// +feature: ui-notification-toast
import type { Meta, StoryObj } from "@storybook/react";
import type { NotificationData } from "@/lib/types/notification";
import { NotificationToast } from "./NotificationToast";

// Presentational card: timers belong to ToastStack, so each state stays on screen here.
const meta: Meta<typeof NotificationToast> = {
  component: NotificationToast,
  title: "UI/NotificationToast",
  args: { onClose: () => {} },
};
export default meta;
type Story = StoryObj<typeof NotificationToast>;

function make(overrides: Partial<NotificationData> = {}): NotificationData {
  return {
    id: "toast-1",
    sessionId: "sess-1",
    sessionName: "my-session",
    message: "Task finished successfully.",
    timestamp: Date.now() - 30_000,
    notificationType: "task_complete",
    onView: () => {},
    ...overrides,
  };
}

export const TaskComplete: Story = { args: { notification: make() } };
export const Info: Story = { args: { notification: make({ notificationType: "info", message: "Session started." }) } };
export const Warning: Story = { args: { notification: make({ notificationType: "warning", message: "Context window is 90% full." }) } };
export const Error: Story = {
  args: { notification: make({ notificationType: "error", priority: "urgent", message: "Session crashed unexpectedly." }) },
};
export const ApprovalNeeded: Story = {
  args: {
    notification: make({
      notificationType: "approval_needed",
      message: "Claude wants to run a command",
      metadata: { approval_id: "appr-1", tool_name: "Bash", tool_input_command: "rm -rf node_modules" },
      sourceApp: "Claude Code",
      sourceWorkingDir: "/Users/you/code/repo",
      onApprove: () => {},
      onDeny: () => {},
      onFocusWindow: () => {},
    }),
  },
};
export const Undo: Story = {
  args: { notification: make({ notificationType: "undo", message: "Session deleted.", onUndo: () => {} }) },
};
export const LongContent: Story = {
  args: {
    notification: make({
      title: "A notification title that is far longer than the toast width can reasonably display",
      message: "A very long message that keeps going to check wrapping behaviour inside the toast body. ".repeat(5),
      sourceApp: "Visual Studio Code",
      sourceWorkingDir: "/Users/you/code/github.com/some-organization/some-deeply-nested-repository/packages/service",
    }),
  },
};
