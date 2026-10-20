// +feature: scheduled-prompts
import type { Meta, StoryObj } from "@storybook/react";
import { PeerHelpGroups, ScheduledPromptsSection } from "./ScheduledPromptsSection";
import { NOW, makePeer, makeState } from "./__fixtures__/scheduledPromptsFixtures";

// The section polls the server; its presentational groups are what the catalog shows.
const meta: Meta<typeof ScheduledPromptsSection> = {
  component: ScheduledPromptsSection,
  title: "UI/ScheduledPromptsSection",
  decorators: [(Story) => <div style={{ width: 420 }}><Story /></div>],
};
export default meta;
type Story = StoryObj<typeof ScheduledPromptsSection>;

export const PeerHelp: Story = {
  render: () => (
    <PeerHelpGroups
      now={NOW}
      state={makeState({ peers: [makePeer(), makePeer({ state: "answered", answeredBy: "diagnose:abc", answer: "Use option B" })] })}
    />
  ),
};
