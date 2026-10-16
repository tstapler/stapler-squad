// +feature: ui-tooltip
import type { Meta, StoryObj } from "@storybook/react";
import { Tooltip } from "./Tooltip";
import { Button } from "./Button";

const meta: Meta<typeof Tooltip> = { component: Tooltip, title: "UI/Tooltip" };
export default meta;
type Story = StoryObj<typeof Tooltip>;

export const Top: Story = { args: { label: "Helpful hint", side: "top", children: <Button>Hover me</Button> } };
export const Bottom: Story = { args: { label: "Below", side: "bottom", children: <Button>Hover me</Button> } };
