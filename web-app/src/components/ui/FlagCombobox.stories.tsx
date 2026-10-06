// +feature: ui-flag-combobox
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import type { FlagOption } from "@/lib/flags/flagTokens";
import { FlagCombobox } from "./FlagCombobox";

// Text input that completes the CLI flag token at the caret. With no flags it is a plain input.
// Type "--" (or part of a flag) to open the listbox.
const meta: Meta<typeof FlagCombobox> = {
  component: FlagCombobox,
  title: "UI/FlagCombobox",
  decorators: [(Story) => <div style={{ width: 420, minHeight: 220 }}><Story /></div>],
};
export default meta;
type Story = StoryObj<typeof FlagCombobox>;

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

function Controlled(props: Partial<React.ComponentProps<typeof FlagCombobox>>) {
  const [v, setV] = useState(props.value ?? "");
  return (
    <FlagCombobox id="story-flags" placeholder="claude --model ..." flags={FLAGS} {...props} value={v} onChange={setV} />
  );
}

export const Empty: Story = { render: () => <Controlled /> };
export const Filled: Story = { render: () => <Controlled value="claude --verbose" /> };
export const PlainInputNoFlags: Story = { render: () => <Controlled flags={[]} value="claude" /> };
export const LongValue: Story = {
  render: () => <Controlled value={"claude " + "--verbose ".repeat(20)} />,
};
