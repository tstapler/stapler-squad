// +feature: scheduled-prompts
import type { Meta, StoryObj } from "@storybook/react";
import { InlineConfirm, InlineError } from "./InlineConfirm";

const meta: Meta<typeof InlineConfirm> = {
  component: InlineConfirm,
  title: "UI/InlineConfirm",
  decorators: [(Story) => <div style={{ width: 420 }}><Story /></div>],
  args: {
    question: "Cancel this scheduled prompt? The agent will not be re-prompted.",
    yesLabel: "Yes, cancel",
    keepLabel: "Keep",
    onYes: () => {},
    onKeep: () => {},
  },
};
export default meta;
type Story = StoryObj<typeof InlineConfirm>;

export const Default: Story = {};
export const Busy: Story = { args: { busy: true } };
export const Failure: Story = {
  render: () => <InlineError message="Couldn't cancel." onRetry={() => {}} onDismiss={() => {}} />,
};
