// +feature: notification-tray
import type { Meta, StoryObj } from "@storybook/react";
import { TrayErrorBoundary } from "./TrayErrorBoundary";

function Broken(): React.ReactElement {
  throw new Error("tray render failed");
}

// A render error inside the tray leaves a 44px link to the Notifications page and the handle.
const meta: Meta<typeof TrayErrorBoundary> = {
  component: TrayErrorBoundary,
  title: "UI/TrayErrorBoundary",
};
export default meta;
type Story = StoryObj<typeof TrayErrorBoundary>;

export const RenderError: Story = { args: { children: <Broken /> } };
