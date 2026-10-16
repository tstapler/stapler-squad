// +feature: ui-unknown-flags-warning
import type { Meta, StoryObj } from "@storybook/react";
import { UnknownFlagsWarning } from "./UnknownFlagsWarning";

const meta: Meta<typeof UnknownFlagsWarning> = {
  component: UnknownFlagsWarning,
  title: "UI/UnknownFlagsWarning",
  args: { id: "unknown-flags", testId: "unknown-flags-warning", program: "claude" },
  decorators: [(Story) => <div style={{ width: 460 }}><Story /></div>],
};
export default meta;
type Story = StoryObj<typeof UnknownFlagsWarning>;

export const SingleFlag: Story = { args: { unknown: ["--experimental"] } };
export const FewFlags: Story = { args: { unknown: ["--foo", "--bar", "--baz"] } };
export const ManyFlagsTruncated: Story = {
  args: { unknown: ["--a", "--b", "--c", "--d", "--e"] },
};
export const LongFlagNameClipped: Story = {
  args: {
    program: "/usr/local/bin/claude",
    unknown: ["--this-flag-name-is-extremely-long-and-gets-clipped-at-forty-characters"],
  },
};
// No unknown flags renders null; the label keeps the canvas non-empty.
export const NoUnknownFlags: Story = {
  render: (args) => (
    <div>
      <span>No unknown flags, nothing below:</span>
      <UnknownFlagsWarning {...args} unknown={[]} />
    </div>
  ),
};
