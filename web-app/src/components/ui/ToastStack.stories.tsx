import { useEffect } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import type { NotificationData } from "@/lib/types/notification";
import { NotificationProvider } from "@/lib/contexts/NotificationContext";
import { useNotificationCommands } from "@/lib/contexts/notificationContexts";

type Seed = Omit<NotificationData, "id" | "timestamp">;

/** Seeds the real provider, which renders the ToastStack. */
function Seeded({ toasts }: { toasts: Seed[] }) {
  const { addNotification } = useNotificationCommands();
  useEffect(() => {
    toasts.forEach(addNotification);
  }, [toasts, addNotification]);
  return <p>The toast stack renders in the corner of the page.</p>;
}

const make = (index: number, overrides: Partial<Seed> = {}): Seed => ({
  sessionId: `sess-${index}`,
  sessionName: `session-${index}`,
  message: "Needs your attention.",
  notificationType: "error",
  isPendingDecision: true,
  ...overrides,
});

const meta: Meta<typeof Seeded> = {
  component: Seeded,
  title: "UI/ToastStack",
  decorators: [
    (Story) => (
      <NotificationProvider>
        <Story />
      </NotificationProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof Seeded>;

export const Single: Story = { args: { toasts: [make(1)] } };
export const Mixed: Story = {
  args: {
    toasts: [
      make(1, { notificationType: "approval_needed", onApprove: () => {}, onDeny: () => {} }),
      make(2, { notificationType: "info", isPendingDecision: false, message: "Session started." }),
      make(3),
    ],
  },
};
