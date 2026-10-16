// +feature: ui-path-completion-dropdown
import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react";
import { PathCompletionDropdown, type CompletionEntry } from "./PathCompletionDropdown";

const entries: CompletionEntry[] = [
  { name: "stapler-squad", path: "/Users/you/code/stapler-squad", isDirectory: true },
  { name: "dotfiles", path: "/Users/you/code/dotfiles", isDirectory: true },
  { name: "README.md", path: "/Users/you/code/README.md", isDirectory: false },
];

const history: CompletionEntry[] = [
  { name: "recent-repo", path: "/Users/you/work/recent-repo", isDirectory: true, isHistory: true },
  { name: "older-repo", path: "/Users/you/work/older-repo", isDirectory: true, isHistory: true },
];

const worktree: CompletionEntry = {
  name: "stapler-squad-feature",
  path: "/Users/you/.stapler-squad/worktrees/stapler-squad-feature",
  isDirectory: true,
  isWorktree: true,
  rootLabel: "stapler-squad",
};

const meta: Meta<typeof PathCompletionDropdown> = {
  component: PathCompletionDropdown,
  title: "UI/PathCompletionDropdown",
  args: { entries, selectedIndex: 0, isLoading: false, onSelect: () => {} },
  decorators: [(Story) => <div style={{ width: 420, position: "relative" }}><Story /></div>],
};
export default meta;
type Story = StoryObj<typeof PathCompletionDropdown>;

export const Default: Story = {};
export const WithHistoryAndDivider: Story = {
  args: { entries: [...history, ...entries], historyCount: history.length, selectedIndex: 1 },
};
export const WithWorktree: Story = { args: { entries: [...entries, worktree], selectedIndex: 3 } };
export const Loading: Story = { args: { entries: [], isLoading: true } };
export const LongNames: Story = {
  args: {
    entries: [
      {
        name: "a-very-long-directory-name-that-keeps-going-and-going-and-going-until-it-overflows",
        path: "/Users/you/a-very-long-directory-name",
        isDirectory: true,
      },
    ],
  },
};
// Empty and not loading renders null; the label keeps the canvas non-empty.
export const Empty: Story = {
  render: (args) => (
    <div>
      <span>No completions, nothing below:</span>
      <PathCompletionDropdown {...args} entries={[]} />
    </div>
  ),
};

function Selectable() {
  const [picked, setPicked] = useState<string>("(none)");
  return (
    <div>
      <PathCompletionDropdown
        entries={entries}
        selectedIndex={0}
        isLoading={false}
        onSelect={(e) => setPicked(e.path)}
      />
      <p>Selected: {picked}</p>
    </div>
  );
}
export const Interactive: Story = { render: () => <Selectable /> };
