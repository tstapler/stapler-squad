// +feature: peer-help-rows
import type { Meta, StoryObj } from "@storybook/react";
import { PeerHelpRow } from "./PeerHelpRow";
import { NOW, makePeer } from "./__fixtures__/scheduledPromptsFixtures";

const meta: Meta<typeof PeerHelpRow> = {
  component: PeerHelpRow,
  title: "UI/PeerHelpRow",
  decorators: [(Story) => <ul style={{ width: 420, listStyle: "none", padding: 0 }}><Story /></ul>],
  args: { info: makePeer(), onChanged: () => {}, now: NOW },
};
export default meta;
type Story = StoryObj<typeof PeerHelpRow>;

export const Waiting: Story = {};
export const Stalled: Story = { args: { info: makePeer({ escalated: true }) } };
export const PeerStopped: Story = {
  args: { info: makePeer({ escalated: true, stateReason: "Peer is stopped. Not delivered unless resumed." }) },
};
export const NotSent: Story = { args: { info: makePeer({ state: "not_sent", stateReason: "peer no longer exists" }) } };
export const Answered: Story = { args: { info: makePeer({ state: "answered", answeredBy: "diagnose:abc", answer: "Use option B" }) } };
export const AnsweredByOperator: Story = {
  args: { info: makePeer({ state: "answered", answeredBy: "operator", onBehalfOf: "diagnose:abc", answer: "Use option B" }) },
};
