// +feature: ui-button
import type { Meta, StoryObj } from "@storybook/react";
import { Button } from "./Button";

const meta: Meta<typeof Button> = { component: Button, title: "UI/Button", args: { children: "Save" } };
export default meta;
type Story = StoryObj<typeof Button>;

export const Primary: Story = { args: { intent: "primary" } };
export const Secondary: Story = { args: { intent: "secondary" } };
export const Danger: Story = { args: { intent: "danger", children: "Delete" } };
export const Ghost: Story = { args: { intent: "ghost" } };
export const Disabled: Story = { args: { disabled: true } };
export const Sizes: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
      <Button size="sm">Small</Button>
      <Button size="md">Medium</Button>
      <Button size="lg">Large</Button>
    </div>
  ),
};
