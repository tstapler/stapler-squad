// +feature: ui-available-flags
import type { Meta, StoryObj } from "@storybook/react";
import type { FlagOption } from "@/lib/flags/flagTokens";
import { AvailableFlags } from "./AvailableFlags";

// "Available flags (N)" disclosure listing CLI flags with info buttons. Renders nothing when empty.
const meta: Meta<typeof AvailableFlags> = {
  component: AvailableFlags,
  title: "UI/AvailableFlags",
  decorators: [(Story) => <div style={{ width: 420 }}><Story /></div>],
};
export default meta;
type Story = StoryObj<typeof AvailableFlags>;

const flag = (name: string, takesValue: boolean, description: string): FlagOption => ({
  name,
  short: "",
  takesValue,
  description,
  aliases: [],
});

const FLAGS: FlagOption[] = [
  flag("--model", true, "Model to use for the current session"),
  flag("--verbose", false, "Enable verbose output"),
  flag("--resume", false, "Resume the most recent conversation"),
];

export const Default: Story = { args: { flags: FLAGS, testId: "available-flags" } };
export const SingleFlag: Story = { args: { flags: FLAGS.slice(0, 1), testId: "available-flags" } };
export const LongDescriptions: Story = {
  args: {
    flags: [
      flag(
        "--append-system-prompt-with-an-unusually-long-flag-name",
        true,
        "A very long description. ".repeat(12),
      ),
      ...FLAGS,
    ],
    testId: "available-flags",
  },
};
// Renders nothing by design; the empty wrapper keeps the story visible in the catalog.
export const EmptyRendersNothing: Story = {
  render: () => (
    <div>
      <em>(AvailableFlags with no flags renders nothing)</em>
      <AvailableFlags flags={[]} testId="available-flags" />
    </div>
  ),
};
