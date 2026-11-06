// +feature: notification-tray
import type { Meta, StoryObj } from "@storybook/react";
import { OfflineBanner } from "./OfflineBanner";

// The one connectivity banner; plain text, never a live region.
const meta: Meta<typeof OfflineBanner> = {
  component: OfflineBanner,
  title: "UI/OfflineBanner",
  decorators: [(Story) => <div style={{ width: 400 }}><Story /></div>],
  args: { online: true, onRetry: () => {} },
};
export default meta;
type Story = StoryObj<typeof OfflineBanner>;

export const Offline: Story = { args: { online: false } };
export const Reconnecting: Story = { args: { online: true, failed: true } };
