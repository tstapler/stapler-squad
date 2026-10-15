// +feature: ui-action-bar
import type { Meta, StoryObj } from "@storybook/react";
import { ActionBar } from "./ActionBar";
import { Button } from "./Button";

// Flex row for page/toolbar actions with gap, justification and overflow variants.
const meta: Meta<typeof ActionBar> = {
  component: ActionBar,
  title: "UI/ActionBar",
  decorators: [(Story) => <div style={{ width: 480, border: "1px dashed #8884" }}><Story /></div>],
};
export default meta;
type Story = StoryObj<typeof ActionBar>;

const actions = (
  <>
    <Button>Save</Button>
    <Button>Cancel</Button>
    <Button>Delete</Button>
  </>
);

export const Default: Story = { render: () => <ActionBar>{actions}</ActionBar> };
export const JustifyEnd: Story = { render: () => <ActionBar justify="end">{actions}</ActionBar> };
export const JustifyBetween: Story = { render: () => <ActionBar justify="between">{actions}</ActionBar> };
export const JustifyCenter: Story = { render: () => <ActionBar justify="center">{actions}</ActionBar> };
export const SmallGap: Story = { render: () => <ActionBar gap="sm">{actions}</ActionBar> };
export const LargeGap: Story = { render: () => <ActionBar gap="lg">{actions}</ActionBar> };
export const SingleAction: Story = { render: () => <ActionBar><Button>Only action</Button></ActionBar> };
export const ManyItemsWrap: Story = {
  render: () => (
    <ActionBar>
      {Array.from({ length: 12 }, (_, i) => <Button key={i}>Action {i + 1}</Button>)}
    </ActionBar>
  ),
};
export const ManyItemsScroll: Story = {
  render: () => (
    <ActionBar scroll>
      {Array.from({ length: 12 }, (_, i) => <Button key={i}>Action {i + 1}</Button>)}
    </ActionBar>
  ),
};
export const Compact: Story = { render: () => <ActionBar compact>{actions}</ActionBar> };
