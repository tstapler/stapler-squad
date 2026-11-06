// +feature: remind-me
import type { Meta, StoryObj } from "@storybook/react";
import { RemindMeMenu } from "./RemindMeMenu";

const meta: Meta<typeof RemindMeMenu> = {
  component: RemindMeMenu,
  title: "UI/RemindMeMenu",
  decorators: [(Story) => <div style={{ width: 320 }}><Story /></div>],
  args: { sessionUuid: "review-1", label: "review:pr-1042 (review): finished" },
};
export default meta;
type Story = StoryObj<typeof RemindMeMenu>;

export const Default: Story = {};
