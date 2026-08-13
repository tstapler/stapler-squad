// +feature: kbd-component
import type { Meta, StoryObj } from "@storybook/react";
import { Kbd } from "./Kbd";

const meta: Meta<typeof Kbd> = {
  component: Kbd,
  title: "UI/Kbd",
};
export default meta;

type Story = StoryObj<typeof Kbd>;

export const Default: Story = {
  args: { children: "K" },
};

export const Enter: Story = {
  args: { children: "Enter", size: "md" },
};

export const Small: Story = {
  args: { children: "⌘", size: "sm" },
};

export const Combo: Story = {
  render: () => (
    <span style={{ display: "flex", alignItems: "center", gap: 4 }}>
      <Kbd>⌘</Kbd>
      <span>+</span>
      <Kbd>K</Kbd>
    </span>
  ),
};
