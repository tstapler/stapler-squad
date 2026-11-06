// +feature: ui-at-command-dropdown
import type { Meta, StoryObj } from "@storybook/react";
import type { WorkflowEntry } from "@/lib/omnibar/detectors/WorkflowDetector";
import { AtCommandDropdown } from "./AtCommandDropdown";

// Workflow suggestions shown when the user types @slug in the omnibar.
const meta: Meta<typeof AtCommandDropdown> = {
  component: AtCommandDropdown,
  title: "UI/AtCommandDropdown",
  decorators: [(Story) => <div style={{ width: 480 }}><Story /></div>],
  args: { selectedIndex: 0, onSelect: () => {} },
};
export default meta;
type Story = StoryObj<typeof AtCommandDropdown>;

const suggestions: WorkflowEntry[] = [
  { slug: "review", name: "Code review", description: "Run a multi-agent review on the current branch" },
  { slug: "fix-bug", name: "Fix bug", description: "Root-cause then fix" },
  { slug: "ship", name: "Ship PR" },
];

export const Default: Story = { args: { suggestions } };
export const SecondSelected: Story = { args: { suggestions, selectedIndex: 1 } };
export const Empty: Story = { args: { suggestions: [] } };
export const LongContent: Story = {
  args: {
    suggestions: [
      {
        slug: "an-extraordinarily-long-workflow-slug",
        name: "A workflow with a very long display name that should not break the layout",
        description: "A long description repeated to test wrapping. ".repeat(4),
      },
    ],
  },
};
