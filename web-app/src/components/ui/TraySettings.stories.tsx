// +feature: notification-tray
import type { Meta, StoryObj } from "@storybook/react";
import { TraySettings } from "./TraySettings";

// Per-device undo window and pinned-card collapse delay.
const meta: Meta<typeof TraySettings> = {
  component: TraySettings,
  title: "UI/TraySettings",
  decorators: [(Story) => <div style={{ width: 400 }}><Story /></div>],
};
export default meta;
type Story = StoryObj<typeof TraySettings>;

export const Default: Story = {};
