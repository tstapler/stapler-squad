// +feature: ui-error-state
import type { Meta, StoryObj } from "@storybook/react";
import { ErrorState } from "./ErrorState";

const meta: Meta<typeof ErrorState> = { component: ErrorState, title: "UI/ErrorState" };
export default meta;
type Story = StoryObj<typeof ErrorState>;

export const Default: Story = { args: { title: "Something broke", message: "Try again in a moment" } };
export const WithRetry: Story = { args: { title: "Load failed", message: "Network error", onRetry: () => {} } };
export const WithDetails: Story = {
  args: { error: new Error("boom"), showDetails: true, onRetry: () => {} },
};
