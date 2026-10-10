// +feature: notification-toast-stack
import type { Meta, StoryObj } from "@storybook/react";
import type { NotificationData } from "@/lib/types/notification";
import { createToastTimerRegistry } from "@/lib/hooks/useToastTimers";
import { ToastStack } from "./ToastStack";

const timers = createToastTimerRegistry();

const meta: Meta<typeof ToastStack> = {
  component: ToastStack,
  title: "UI/ToastStack",
  args: { timers, onRemove: () => {} },
};
export default meta;
type Story = StoryObj<typeof ToastStack>;

function make(id: string, overrides: Partial<NotificationData> = {}): NotificationData {
  return {
    id,
    sessionId: `sess-${id}`,
    sessionName: `session-${id}`,
    message: "Needs your attention.",
    timestamp: Date.now() - 30_000,
    notificationType: "error",
    isPendingDecision: true,
    ...overrides,
  };
}

export const Single: Story = { args: { toasts: [make("1")] } };
export const Mixed: Story = {
  args: {
    toasts: [
      make("1", { notificationType: "approval_needed", onApprove: () => {}, onDeny: () => {} }),
      make("2", { notificationType: "info", isPendingDecision: false, message: "Session started." }),
      make("3"),
    ],
  },
};
