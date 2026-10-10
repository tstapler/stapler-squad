// +feature: notification-tray
import type { Meta, StoryObj } from "@storybook/react";
import { TrayEntryChip } from "./TrayHandle";

// The handle itself is flag-gated and renders nothing without a provider; the
// phone entry chip is the visible, prop-driven piece.
const meta: Meta<typeof TrayEntryChip> = {
  component: TrayEntryChip,
  title: "UI/TrayEntryChip",
  decorators: [(Story) => <div style={{ width: 360 }}><Story /></div>],
  args: { chipText: "+3 more", chipLabel: "3 more notifications, open tray", onOpen: () => {} },
};
export default meta;
type Story = StoryObj<typeof TrayEntryChip>;

export const Bell: Story = { args: { content: "bell" } };
export const MoreRow: Story = { args: { content: "more-row" } };
export const KeyboardChip: Story = { args: { content: "keyboard-chip", chipText: "3 notifications - 1 needs you" } };
