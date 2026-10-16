// +feature: ui-slash-command-dropdown
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import type { SlashCommandInfo } from "@/lib/hooks/useSlashCommands";
import { SlashCommandDropdown } from "./SlashCommandDropdown";

const cmd = (c: Partial<SlashCommandInfo> & { name: string }) =>
  ({ title: "", description: "", source: "", ...c }) as SlashCommandInfo;

const suggestions: SlashCommandInfo[] = [
  cmd({ name: "help", description: "Show available commands", source: "builtin" }),
  cmd({ name: "code:fix-loop", title: "Fix loop", description: "Iterate until checks pass", source: "project" }),
  cmd({ name: "notes", description: "Open personal notes", source: "user" }),
  cmd({ name: "bare" }),
];

const meta: Meta<typeof SlashCommandDropdown> = {
  component: SlashCommandDropdown,
  title: "UI/SlashCommandDropdown",
  args: { suggestions, selectedIndex: 0, onSelect: () => {} },
  decorators: [(Story) => <div style={{ width: 460, position: "relative" }}><Story /></div>],
};
export default meta;
type Story = StoryObj<typeof SlashCommandDropdown>;

export const Default: Story = {};
export const SecondSelected: Story = { args: { selectedIndex: 1 } };
export const NoMatches: Story = { args: { suggestions: [] } };
export const LongDescription: Story = {
  args: {
    suggestions: [
      cmd({
        name: "sdd:full",
        description:
          "Full SDD workflow: ideate, research, plan, validate, implement, verify, and ship with parallel agents at each phase",
        source: "user",
      }),
    ],
  },
};

function Selectable() {
  const [idx, setIdx] = useState(0);
  const [picked, setPicked] = useState("(none)");
  return (
    <div>
      <button type="button" onClick={() => setIdx((i) => (i + 1) % suggestions.length)}>Next</button>
      <SlashCommandDropdown
        suggestions={suggestions}
        selectedIndex={idx}
        onSelect={(c) => setPicked(c.name)}
      />
      <p>Selected: {picked}</p>
    </div>
  );
}
export const Interactive: Story = { render: () => <Selectable /> };
