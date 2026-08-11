// +feature: keyboard-shortcut-overlay
import React, { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { KeyboardShortcutOverlay } from "./KeyboardShortcutOverlay";

const meta: Meta<typeof KeyboardShortcutOverlay> = {
  component: KeyboardShortcutOverlay,
  title: "UI/KeyboardShortcutOverlay",
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof KeyboardShortcutOverlay>;

export const Open: Story = {
  args: {
    isOpen: true,
    onClose: () => {},
  },
};

function ControlledStory() {
  const [open, setOpen] = React.useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>Open Shortcuts (?)</button>
      <KeyboardShortcutOverlay isOpen={open} onClose={() => setOpen(false)} />
    </>
  );
}

export const Controlled: Story = {
  render: () => <ControlledStory />,
};
