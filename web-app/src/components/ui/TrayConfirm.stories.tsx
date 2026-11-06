// +feature: notification-tray
import type { Meta, StoryObj } from "@storybook/react";
import { TrayConfirm } from "./TrayConfirm";

// Inline confirm for the tray's destructive actions; focus starts on Cancel.
const meta: Meta<typeof TrayConfirm> = {
  component: TrayConfirm,
  title: "UI/TrayConfirm",
  decorators: [(Story) => <div style={{ width: 400 }}><Story /></div>],
  args: {
    text: "Clear 20 read notifications? This can't be undone. 2 awaiting decision kept.",
    confirmLabel: "Clear 20",
    onConfirm: () => {},
    onCancel: () => {},
  },
};
export default meta;
type Story = StoryObj<typeof TrayConfirm>;

export const Default: Story = {};
export const AfterFailure: Story = { args: { error: true } };
