// +feature: ui-input
import type { Meta, StoryObj } from "@storybook/react";
import { Input } from "./Input";

const meta: Meta<typeof Input> = { component: Input, title: "UI/Input", args: { placeholder: "Type here" } };
export default meta;
type Story = StoryObj<typeof Input>;

export const Default: Story = {};
export const WithLabel: Story = { args: { label: "Name", id: "story-name" } };
export const WithError: Story = { args: { label: "Name", id: "story-name-err", error: "Name is required" } };
export const Disabled: Story = { args: { disabled: true, value: "Locked" } };
