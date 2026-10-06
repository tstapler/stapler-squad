// +feature: ui-app-link
import type { Meta, StoryObj } from "@storybook/react";
import { AppLink } from "./AppLink";

// next/link wrapper with prefetch disabled by default.
const meta: Meta<typeof AppLink> = {
  component: AppLink,
  title: "UI/AppLink",
};
export default meta;
type Story = StoryObj<typeof AppLink>;

export const Default: Story = { render: () => <AppLink href="/sessions">Sessions</AppLink> };
export const LongLabel: Story = {
  render: () => (
    <div style={{ width: 240 }}>
      <AppLink href="/history">A very long link label that has to wrap across several lines in a narrow column</AppLink>
    </div>
  ),
};
export const WithPrefetch: Story = {
  render: () => <AppLink href="/review-queue" prefetch>Review queue (prefetch enabled)</AppLink>,
};
