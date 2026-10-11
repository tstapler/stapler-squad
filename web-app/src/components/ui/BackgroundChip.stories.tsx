// +feature: ui-background-chip
import type { Meta, StoryObj } from "@storybook/react";
import { BackgroundChip } from "./BackgroundChip";

// Text chip that marks a notification whose session is hidden (Background).
const meta: Meta<typeof BackgroundChip> = { component: BackgroundChip, title: "UI/BackgroundChip" };
export default meta;
type Story = StoryObj<typeof BackgroundChip>;

export const Default: Story = {};
