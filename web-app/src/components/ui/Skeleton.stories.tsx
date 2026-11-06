// +feature: ui-skeleton
import type { Meta, StoryObj } from "@storybook/react";
import { Skeleton } from "./Skeleton";

const meta: Meta<typeof Skeleton> = { component: Skeleton, title: "UI/Skeleton" };
export default meta;
type Story = StoryObj<typeof Skeleton>;

export const Text: Story = { args: { variant: "text", width: 200 } };
export const Circular: Story = { args: { variant: "circular", width: 40, height: 40 } };
export const Rectangular: Story = { args: { variant: "rectangular", width: 200, height: 80 } };
