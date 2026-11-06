// +feature: ui-flag-info-button
import type { Meta, StoryObj } from "@storybook/react";
import { FlagInfoButton } from "./FlagInfoButton";

// Tap-to-toggle inline flag description. Renders nothing without a description.
const meta: Meta<typeof FlagInfoButton> = {
  component: FlagInfoButton,
  title: "UI/FlagInfoButton",
  decorators: [(Story) => <div style={{ width: 420 }}><Story /></div>],
};
export default meta;
type Story = StoryObj<typeof FlagInfoButton>;

export const Default: Story = {
  args: { flag: "--model", description: "Model to use for the current session." },
};
export const LongDescription: Story = {
  args: {
    flag: "--dangerously-skip-permissions",
    description:
      "Bypass all permission checks. Recommended only for sandboxes with no internet access, because the agent can run any command without confirmation and may modify or delete files outside the working directory.",
  },
};
// With no description the component renders null; a label keeps the canvas non-empty.
export const NoDescription: Story = {
  render: () => (
    <div>
      <span>Flag --verbose (no description, so no info button): </span>
      <FlagInfoButton flag="--verbose" description="" />
    </div>
  ),
};
