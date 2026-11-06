// +feature: ui-badge
import type { Meta, StoryObj } from "@storybook/react";
import { Badge } from "./Badge";

const meta: Meta<typeof Badge> = { component: Badge, title: "UI/Badge", args: { children: "Badge" } };
export default meta;
type Story = StoryObj<typeof Badge>;

export const Default: Story = { args: { intent: "default" } };
export const Intents: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 8 }}>
      {(["default", "success", "warning", "error", "critical", "primary"] as const).map((i) => (
        <Badge key={i} intent={i}>{i}</Badge>
      ))}
    </div>
  ),
};
export const Small: Story = { args: { size: "sm" } };
