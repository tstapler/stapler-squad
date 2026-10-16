// +feature: ui-navigation
import type { Meta, StoryObj } from "@storybook/react";
import { Navigation } from "./Navigation";

// Reads the pathname from next/navigation and the "backlog" feature flag from
// FeatureFlagsContext (defaults to off without a provider, so the Backlog link
// is hidden here).
const meta: Meta<typeof Navigation> = {
  component: Navigation,
  title: "UI/Navigation",
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof Navigation>;

export const Default: Story = {};
