// +feature: ui-notification-panel
import { useEffect } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { NotificationProvider, useNotifications } from "@/lib/contexts/NotificationContext";
import type { NotificationData } from "@/lib/types/notification";
import { NotificationPanel } from "./NotificationPanel";

// The panel reads NotificationContext, whose value is not injectable, so each story mounts the
// real NotificationProvider and seeds history through its API. Offline, the backend history
// fetch fails and leaves the seeded items in place.
const meta: Meta<typeof NotificationPanel> = {
  component: NotificationPanel,
  title: "UI/NotificationPanel",
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof NotificationPanel>;

type Seed = Omit<NotificationData, "id" | "timestamp">;

function OpenAndSeed({ seeds }: { seeds: Seed[] }) {
  const { addToHistoryOnly, togglePanel } = useNotifications();
  useEffect(() => {
    seeds.forEach(addToHistoryOnly);
    togglePanel();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return null;
}

function Harness({ seeds }: { seeds: Seed[] }) {
  return (
    <NotificationProvider>
      <OpenAndSeed seeds={seeds} />
      <NotificationPanel />
    </NotificationProvider>
  );
}

const base = { sessionId: "sess-1", sessionName: "my-session" };

export const Empty: Story = { render: () => <Harness seeds={[]} /> };

export const Mixed: Story = {
  render: () => (
    <Harness
      seeds={[
        { ...base, message: "Task finished successfully.", notificationType: "task_complete" },
        { ...base, sessionId: "sess-2", sessionName: "other", message: "Disk space is low.", notificationType: "warning" },
        { ...base, sessionId: "sess-3", sessionName: "broken", message: "Session crashed.", notificationType: "error" },
        {
          ...base,
          sessionId: "sess-4",
          sessionName: "needs-you",
          message: "Claude wants to run a command",
          notificationType: "approval_needed",
          metadata: { approval_id: "appr-1", tool_name: "Bash", tool_input_command: "git push --force" },
        },
      ]}
    />
  ),
};

export const LongContent: Story = {
  render: () => (
    <Harness
      seeds={Array.from({ length: 12 }, (_, i) => ({
        ...base,
        sessionId: `sess-${i}`,
        sessionName: `session-with-a-rather-long-name-${i}`,
        message: "A long notification message that wraps across several lines in the compact panel. ".repeat(3),
        notificationType: "info" as const,
      }))}
    />
  ),
};
