// +feature: scheduled-prompts
import type { Meta, StoryObj } from "@storybook/react";
import { ScheduledPrompts } from "./ScheduledPrompts";
import { MIN, NOW, makePeer, makeState, makeTrigger } from "./__fixtures__/scheduledPromptsFixtures";

const triggers = [
  makeTrigger({ ownerTitle: "diagnose:abc", nextFireAt: NOW + 25 * MIN }),
  makeTrigger({ ownerTitle: "review:pr-1042", triggerType: "ci_event", condition: "when PR #901 (o/r) CI passes", nextFireAt: undefined }),
];

const meta: Meta<typeof ScheduledPrompts> = {
  component: ScheduledPrompts,
  title: "UI/ScheduledPrompts",
  decorators: [(Story) => <div style={{ width: 420 }}><Story /></div>],
  args: { state: makeState({ triggers, peers: [makePeer()] }), resumeFlag: async () => {}, now: NOW },
};
export default meta;
type Story = StoryObj<typeof ScheduledPrompts>;

export const Default: Story = {};
export const Empty: Story = { args: { state: makeState() } };
export const Paused: Story = { args: { state: makeState({ enabled: false, triggers }) } };
export const LoadFailed: Story = { args: { state: makeState({ failed: true, lastUpdatedAt: null }) } };
export const Missed: Story = {
  args: { state: makeState({ triggers: [makeTrigger({ status: "missed", endReason: "expired" }), ...triggers] }) },
};
