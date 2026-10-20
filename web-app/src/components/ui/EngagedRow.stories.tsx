import type { Meta, StoryObj } from "@storybook/react";
import { EngagedRow } from "./EngagedRow";

// A Needs-you card as the engagement tracker sees it; without a tracker it is a plain listitem.
const meta: Meta<typeof EngagedRow> = {
  component: EngagedRow,
  title: "UI/EngagedRow",
  decorators: [(Story) => <div role="list" style={{ width: 420 }}><Story /></div>],
  args: { cardId: "n1", role: "listitem", tabIndex: 0, children: <p>Approve `git push`?</p> },
};
export default meta;
type Story = StoryObj<typeof EngagedRow>;

export const Default: Story = {};
export const PreExpanded: Story = { args: { preExpand: true } };
