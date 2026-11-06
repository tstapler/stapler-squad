// +feature: ui-keyboard-hint
import type { Meta, StoryObj } from "@storybook/react";
import { KeyboardHint, KeyboardHints } from "./KeyboardHint";

const meta: Meta<typeof KeyboardHint> = {
  component: KeyboardHint,
  title: "UI/KeyboardHint",
};
export default meta;
type Story = StoryObj<typeof KeyboardHint>;

export const SingleKey: Story = { args: { keys: "Esc", description: "Close dialog" } };
export const Combination: Story = { args: { keys: ["Cmd", "K"], description: "Open command palette" } };
export const LongDescription: Story = {
  args: {
    keys: ["Ctrl", "Shift", "Enter"],
    description: "Submit the form and immediately start the session without opening the review step",
  },
};
export const HintList: Story = {
  render: () => (
    <KeyboardHints
      title="Keyboard shortcuts"
      hints={[
        { keys: ["Cmd", "K"], description: "Command palette" },
        { keys: "Esc", description: "Close" },
        { keys: ["Ctrl", "Enter"], description: "Submit" },
      ]}
    />
  ),
};
export const EmptyHintList: Story = {
  render: () => <KeyboardHints title="No shortcuts available" hints={[]} />,
};
