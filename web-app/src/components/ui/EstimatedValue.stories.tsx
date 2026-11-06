// +feature: ui-estimated-value
import type { Meta, StoryObj } from "@storybook/react";
import { EstimatedValue } from "./EstimatedValue";

// Marks a modeled/heuristic number with a leading "~" and an accessible explanation.
const meta: Meta<typeof EstimatedValue> = {
  component: EstimatedValue,
  title: "UI/EstimatedValue",
};
export default meta;
type Story = StoryObj<typeof EstimatedValue>;

export const Default: Story = {
  args: { children: "$5.00", title: "Estimated from token counts and list pricing; not billed usage." },
};
export const Percentage: Story = {
  args: { children: "42%", title: "Heuristic cache ROI estimate." },
};
export const LongContent: Story = {
  args: {
    children: "$1,234,567,890.12 across all sessions and tools",
    title: "Modeled value. ".repeat(15),
  },
};
export const InSentence: Story = {
  render: () => (
    <p>
      This session cost <EstimatedValue title="Modeled from token usage">$0.87</EstimatedValue> so far.
    </p>
  ),
};
